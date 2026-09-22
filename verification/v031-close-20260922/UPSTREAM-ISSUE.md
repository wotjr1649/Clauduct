# Loopback TCP send-only shutdown causes premature receive EOF while peer remains open

Environment observed on 2026-09-22:

- Windows 11 Pro, build 26200, x64.
- AdGuard service file version 8.0.5570 (release installer build 8.0.1.5570).
- AdGuard.Core.dll 1.22.29.
- Loaded AdGuardNetLibWfp.dll 8.0.91.0; installed adgnetworkwfpdrv.sys 8.0.91.0.
- Protection enabled throughout this reproduction. No exclusions or filtering settings changed.

This single-threaded C reproduction uses only Winsock and 127.0.0.1. It sends six
public bytes from the accepted server socket. Before observing the server receive
result, the client never calls send, shutdown, or closesocket.

Expected: shutdown(server, SD_SEND) ends only the server's sending direction.
The server's recv must remain pending until its 500 ms receive timeout because
the client is still open and has sent no data or FIN.

Observed:

```text
No shutdown:   recv=-1, WSAETIMEDOUT=10060, 513160 us.
SD_SEND only:  recv=0 (EOF), error=0, 2476 us.
Client shutdown calls=0; client close calls=0 at observation.
Before: server ESTABLISHED, client ESTABLISHED.
After premature EOF: server LAST_ACK, client ESTABLISHED.
Both TCP entries still belong to the reproduction process.
```

A preceding run without TCP-table instrumentation reproduced the same issue:
no-shutdown timed out at 512180 us; send-only shutdown returned EOF at 4451 us.

Separately, standard Node HTTP, .NET HttpClient, and Go TCP tests have reproduced
ECONNRESET, response truncation, and lost data during immediate socket closure.
The unchanged Node 31-case, .NET 35-case, and Go 400-connection suites previously
passed with protection disabled and failed with protection enabled. This is
supporting evidence, not an Off comparison for the C reproduction: the C test
has only been run with protection enabled.

Please investigate whether the filtering path converts a half-close into a
bidirectional close or injects an early disconnect. We have not identified the
specific internal callback or established that every reset has this same cause.
Can you provide a fixed build or a narrowly scoped diagnostic procedure for
this synthetic loopback connection, without disabling protection or excluding
localhost/apps?

Build with MinGW-w64 GCC:

```text
gcc -Wall -Wextra -Werror socket-half-close.c -lws2_32 -liphlpapi -o probe.exe
probe.exe
```

Exit 0 requires both receive-direction checks to time out normally; premature
EOF returns exit 1. TCP-table output is restricted to this reproduction's
loopback ports. No credentials, external endpoints, or private project data are
used.

```c
// Public loopback-only diagnostic. No credentials, files or filter changes.
// Build: gcc -Wall -Wextra -Werror socket-half-close.c -lws2_32 -liphlpapi -o probe.exe
// Both connected sockets remain open until the receive result is recorded.
#include <winsock2.h>
#include <windows.h>
#include <iphlpapi.h>
#include <stdio.h>

// Retain only this probe's two connected tuples. Never dump the host TCP table.
static int snapshot(const char *phase, unsigned short client_port, unsigned short server_port) {
    DWORD size = 0;
    if (GetExtendedTcpTable(NULL, &size, FALSE, AF_INET, TCP_TABLE_OWNER_PID_ALL, 0) != ERROR_INSUFFICIENT_BUFFER ||
        size > 16 * 1024 * 1024) return 0;
    MIB_TCPTABLE_OWNER_PID *table = HeapAlloc(GetProcessHeap(), 0, size);
    if (table == NULL) return 0;
    DWORD status = GetExtendedTcpTable(table, &size, FALSE, AF_INET, TCP_TABLE_OWNER_PID_ALL, 0);
    if (status == NO_ERROR) {
        for (DWORD i = 0; i < table->dwNumEntries; ++i) {
            MIB_TCPROW_OWNER_PID *row = &table->table[i];
            unsigned short local = ntohs((unsigned short)row->dwLocalPort);
            unsigned short remote = ntohs((unsigned short)row->dwRemotePort);
            if (row->dwLocalAddr != htonl(INADDR_LOOPBACK) || row->dwRemoteAddr != htonl(INADDR_LOOPBACK)) continue;
            if (local != client_port && remote != client_port && local != server_port && remote != server_port) continue;
            printf("phase=%s local_port=%hu remote_port=%hu owner_pid=%lu state=%lu\n",
                   phase, local, remote, (unsigned long)row->dwOwningPid, (unsigned long)row->dwState);
        }
    }
    HeapFree(GetProcessHeap(), 0, table);
    return status == NO_ERROR;
}

int main(void) {
    WSADATA data;
    if (WSAStartup(MAKEWORD(2, 2), &data) != 0) return 2;
    int failures = 0;
    for (int half_close = 0; half_close <= 1; ++half_close) {
        SOCKET listener = INVALID_SOCKET, client = INVALID_SOCKET, server = INVALID_SOCKET;
        struct sockaddr_in address = {0};
        int length = sizeof(address), setup_error = 0;
        DWORD timeout_ms = 500;
        char byte;
        LARGE_INTEGER start, finish, frequency;
        QueryPerformanceFrequency(&frequency);
        address.sin_family = AF_INET;
        address.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
        listener = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
        if (listener == INVALID_SOCKET ||
            bind(listener, (struct sockaddr *)&address, sizeof(address)) != 0 ||
            listen(listener, 1) != 0 ||
            getsockname(listener, (struct sockaddr *)&address, &length) != 0) {
            setup_error = 1; goto cleanup;
        }
        client = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
        if (client == INVALID_SOCKET || connect(client, (struct sockaddr *)&address, length) != 0) {
            setup_error = 1; goto cleanup;
        }
        server = accept(listener, NULL, NULL);
        if (server == INVALID_SOCKET ||
            setsockopt(server, SOL_SOCKET, SO_RCVTIMEO, (const char *)&timeout_ms, sizeof(timeout_ms)) != 0 ||
            send(server, "PUBLIC", 6, 0) != 6) {
            setup_error = 1; goto cleanup;
        }
        struct sockaddr_in client_address = {0}, server_peer = {0};
        int endpoint_length = sizeof(client_address);
        if (getsockname(client, (struct sockaddr *)&client_address, &endpoint_length) != 0 ||
            getpeername(server, (struct sockaddr *)&server_peer, &endpoint_length) != 0) {
            setup_error = 1; goto cleanup;
        }
        printf("half_close=%d probe_pid=%lu client_port=%hu server_port=%hu server_peer_port=%hu\n",
               half_close, (unsigned long)GetCurrentProcessId(), ntohs(client_address.sin_port),
               ntohs(address.sin_port), ntohs(server_peer.sin_port));
        if (!snapshot("before", ntohs(client_address.sin_port), ntohs(address.sin_port))) {
            setup_error = 1; goto cleanup;
        }
        if (half_close && shutdown(server, SD_SEND) != 0) {
            setup_error = 1; goto cleanup;
        }
        QueryPerformanceCounter(&start);
        int received = recv(server, &byte, 1, 0);
        int error = received == SOCKET_ERROR ? WSAGetLastError() : 0;
        QueryPerformanceCounter(&finish);
        // The client has sent nothing and has NEVER called shutdown/closesocket.
        // The server's send-only shutdown must leave its receive direction open.
        int passed = received == SOCKET_ERROR && error == WSAETIMEDOUT;
        printf("half_close=%d received=%d error=%d elapsed_us=%lld client_shutdowns=0 client_closes=0 result=%s\n",
               half_close, received, error,
               (long long)((finish.QuadPart - start.QuadPart) * 1000000 / frequency.QuadPart),
               passed ? "PASS" : "FAIL");
        failures += !passed;
        if (!snapshot("after", ntohs(client_address.sin_port), ntohs(address.sin_port))) {
            setup_error = 1;
        }
cleanup:
        if (setup_error) { printf("setup_error=%d\n", WSAGetLastError()); ++failures; }
        if (server != INVALID_SOCKET) closesocket(server);
        if (client != INVALID_SOCKET) closesocket(client);
        if (listener != INVALID_SOCKET) closesocket(listener);
    }
    WSACleanup();
    return failures ? 1 : 0;
}
```

Winsock contract: [Microsoft shutdown documentation](https://learn.microsoft.com/en-us/windows/win32/api/winsock/nf-winsock-shutdown).
