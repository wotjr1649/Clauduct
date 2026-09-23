// Read-only WFP metadata and public loopback socket diagnostics. No filter writes.
// gcc -Wall -Wextra -Werror socket-shutdown-path.c -lws2_32 -liphlpapi -lfwpuclnt -luuid -o path.exe
// Reuse the published reproduction's scoped TCP-table observer unchanged.
#define main published_reproduction_main
#include "socket-half-close.c"
#undef main
#include <fwpmu.h>
#include <wchar.h>

// MinGW omits these GUID declarations; values verified in Windows SDK 10.0.26100.0 fwpmu.h.
static const GUID stream_v4 = {0x3b89653c, 0xc170, 0x49e4, {0xb1,0xcd,0xe0,0xee,0xee,0xe1,0x9a,0x3e}};
static const GUID stream_v6 = {0x47c9137a, 0x7ec4, 0x46b3, {0xb6,0xe4,0x48,0xe9,0x26,0xb1,0xed,0xa4}};

static int mentions(const wchar_t *value, const wchar_t *word) {
    if (!value) return 0;
    for (size_t i = 0; value[i] && i < 4096; ++i) if (_wcsnicmp(value+i, word, wcslen(word)) == 0) return 1;
    return 0;
}

static void guid(const GUID *g) {
    printf("%08lx-%04x-%04x-%02x%02x-", (unsigned long)g->Data1, g->Data2, g->Data3, g->Data4[0], g->Data4[1]);
    for (int i = 2; i < 8; ++i) printf("%02x", g->Data4[i]);
}

static int wfp_metadata(void) {
    HANDLE engine = NULL, iterator = NULL;
    DWORD status = FwpmEngineOpen0(NULL, RPC_C_AUTHN_WINNT, NULL, NULL, &engine);
    if (status != ERROR_SUCCESS) { printf("wfp_open_error=%lu\n", (unsigned long)status); return 0; }
    status = FwpmCalloutCreateEnumHandle0(engine, NULL, &iterator);
    unsigned int matched = 0;
    for (int page = 0; status == ERROR_SUCCESS && page < 8; ++page) {
        FWPM_CALLOUT0 **entries = NULL;
        UINT32 count = 0;
        status = FwpmCalloutEnum0(engine, iterator, 128, &entries, &count);
        if (status != ERROR_SUCCESS) break;
        for (UINT32 i = 0; i < count; ++i) {
            FWPM_CALLOUT0 *entry = entries[i];
            FWPM_PROVIDER0 *provider = NULL;
            int stream = IsEqualGUID(&entry->applicableLayer, &stream_v4) || IsEqualGUID(&entry->applicableLayer, &stream_v6);
            DWORD result = entry->providerKey ? FwpmProviderGetByKey0(engine, entry->providerKey, &provider) : ERROR_SUCCESS;
            if (result != ERROR_SUCCESS) { status = result; break; }
            // Emit fixed attribution and IDs only, never arbitrary rule names/conditions.
            int adguard = (provider && mentions(provider->displayData.name, L"Adguard")) ||
                mentions(entry->displayData.name, L"Adguard") || mentions(entry->displayData.name, L"adgnetwork");
            if (adguard || stream) {
                const char *layer = IsEqualGUID(&entry->applicableLayer, &stream_v4) ? "STREAM_V4" :
                    IsEqualGUID(&entry->applicableLayer, &stream_v6) ? "STREAM_V6" : "OTHER";
                printf("provider=%s callout_id=%u flags=%u layer=%s callout_key=", adguard ? "AdGuard" : "UNATTRIBUTED", entry->calloutId, entry->flags, layer);
                guid(&entry->calloutKey);
                printf(" layer_key="); guid(&entry->applicableLayer);
                printf(" provider_key_present=%d name_mentions_netfilter=%d\n", entry->providerKey != NULL,
                       mentions(entry->displayData.name, L"NetFilter"));
                matched += adguard;
            }
            if (provider) FwpmFreeMemory0((void **)&provider);
        }
        FwpmFreeMemory0((void **)&entries);
        if (count < 128) break;
        if (page == 7) status = ERROR_MORE_DATA;
    }
    if (iterator) FwpmCalloutDestroyEnumHandle0(engine, iterator);
    FwpmEngineClose0(engine);
    printf("wfp_query_error=%lu adguard_callouts=%u filter_changes=0\n", (unsigned long)status, matched);
    return status == ERROR_SUCCESS;
}

static int socket_metadata(SOCKET s) {
    WSAPROTOCOL_INFOA info = {0};
    int size = sizeof(info);
    struct linger linger_value = {0};
    int linger_size = sizeof(linger_value);
    if (getsockopt(s, SOL_SOCKET, SO_PROTOCOL_INFOA, (char *)&info, &size) != 0 ||
        getsockopt(s, SOL_SOCKET, SO_LINGER, (char *)&linger_value, &linger_size) != 0) return 0;
    printf("catalog=%lu chain_length=%d provider=", (unsigned long)info.dwCatalogEntryId, info.ProtocolChain.ChainLen);
    guid(&info.ProviderId);
    printf(" linger_enabled=%hu linger_seconds=%hu SD_SEND=%d\n", linger_value.l_onoff, linger_value.l_linger, SD_SEND);
    return 1;
}

static int (*observe_before_shutdown)(void);

static int trial(int from_client, int bytes, int consume_first) {
    SOCKET listener = INVALID_SOCKET, client = INVALID_SOCKET, server = INVALID_SOCKET;
    struct sockaddr_in address = {0}, client_address = {0};
    int length = sizeof(address), failed = 2;
    DWORD timeout = 500;
    char buffer[6];
    LARGE_INTEGER frequency, start, finish;
    QueryPerformanceFrequency(&frequency);
    address.sin_family = AF_INET;
    address.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
    listener = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
    if (listener == INVALID_SOCKET || bind(listener, (struct sockaddr *)&address, length) != 0 ||
        listen(listener, 1) != 0 || getsockname(listener, (struct sockaddr *)&address, &length) != 0) goto cleanup;
    client = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
    if (client == INVALID_SOCKET || connect(client, (struct sockaddr *)&address, length) != 0) goto cleanup;
    server = accept(listener, NULL, NULL);
    if (server == INVALID_SOCKET || getsockname(client, (struct sockaddr *)&client_address, &length) != 0) goto cleanup;
    if (setsockopt(server, SOL_SOCKET, SO_RCVTIMEO, (const char *)&timeout, sizeof(timeout)) != 0 ||
        setsockopt(client, SOL_SOCKET, SO_RCVTIMEO, (const char *)&timeout, sizeof(timeout)) != 0) goto cleanup;
    SOCKET sender = from_client ? client : server, receiver = from_client ? server : client;
    printf("case sender=%s bytes=%d consume_first=%d\n", from_client ? "client" : "server", bytes, consume_first);
    if (!socket_metadata(sender)) goto cleanup;
    if (bytes && send(sender, "PUBLIC", bytes, 0) != bytes) goto cleanup;
    if (consume_first && recv(receiver, buffer, bytes, MSG_WAITALL) != bytes) goto cleanup;
    if (observe_before_shutdown && !observe_before_shutdown()) goto cleanup;
    QueryPerformanceCounter(&start);
    printf("phase=before_shutdown qpc=%lld\n", (long long)start.QuadPart);
    if (shutdown(sender, SD_SEND) != 0) goto cleanup;
    QueryPerformanceCounter(&start);
    int n = recv(sender, buffer, 1, 0), error = n == SOCKET_ERROR ? WSAGetLastError() : 0;
    QueryPerformanceCounter(&finish);
    printf("sender_receive=%d error=%d elapsed_us=%lld peer_shutdowns=0 peer_closes=0 qpc=%lld\n", n, error,
           (long long)((finish.QuadPart-start.QuadPart)*1000000/frequency.QuadPart), (long long)finish.QuadPart);
    failed = !(n == SOCKET_ERROR && error == WSAETIMEDOUT);
    if (!snapshot("after", ntohs(client_address.sin_port), ntohs(address.sin_port))) failed = 1;
cleanup:
    if (failed == 2) printf("trial_setup_or_observer_error=1\n");
    QueryPerformanceCounter(&finish);
    printf("phase=before_cleanup qpc=%lld\n", (long long)finish.QuadPart);
    if (server != INVALID_SOCKET) closesocket(server);
    if (client != INVALID_SOCKET) closesocket(client);
    if (listener != INVALID_SOCKET) closesocket(listener);
    return failed;
}

#ifndef SOCKET_PATH_NO_MAIN
int main(void) {
    WSADATA data;
    if (WSAStartup(MAKEWORD(2, 2), &data) != 0) return 2;
    int failures = !wfp_metadata();
    if (GetEnvironmentVariableA("CLAUDUCT_WFP_METADATA_ONLY", NULL, 0)) { WSACleanup(); return failures ? 1 : 0; }
    for (int sender = 0; sender <= 1; ++sender) {
        failures += trial(sender, 0, 0);
        failures += trial(sender, 6, 0);
        failures += trial(sender, 6, 1);
    }
    WSACleanup();
    printf("diagnostic_failures=%d\n", failures);
    return failures ? 1 : 0;
}
#endif
