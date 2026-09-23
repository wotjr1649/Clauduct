// Task-owned, 15-second, in-memory ETW observation; never changes filtering policy.
// First learn this probe's TCBs from PID-filtered connections. Then retain only their
// numeric state/injection events in this callback. No packet bytes or disk trace.
// gcc -Wall -Wextra -Werror socket-shutdown-etw.c -lws2_32 -liphlpapi -lfwpuclnt -luuid -ladvapi32 -ltdh -o etw.exe
#define SOCKET_PATH_NO_MAIN
#include "socket-shutdown-path.c"
#include <evntrace.h>
#include <evntcons.h>
#include <tdh.h>
#include <stdint.h>

// Declarations absent from MinGW headers, verified against Windows SDK 10.0.26100.0.
typedef struct { LPWSTR FieldName; USHORT CompareOp; LPWSTR Value; } PAYLOAD_FILTER_PREDICATE;
ULONG WINAPI TdhCreatePayloadFilter(LPCGUID, const EVENT_DESCRIPTOR *, BOOLEAN, ULONG, PAYLOAD_FILTER_PREDICATE *, void **);
ULONG WINAPI TdhDeletePayloadFilter(void **);
ULONG WINAPI TdhAggregatePayloadFilters(ULONG, void **, BOOLEAN *, EVENT_FILTER_DESCRIPTOR *);
ULONG WINAPI TdhCleanupPayloadEventFilterDescriptor(EVENT_FILTER_DESCRIPTOR *);

static const GUID tcpip = {0x2f07e2ee,0x15db,0x40f1,{0x90,0xef,0x9d,0x7b,0xa2,0x82,0x18,0x8a}};
static struct { BOOLEAN FilterIn; UCHAR Reserved; USHORT Count; USHORT Events[11]; } event_ids = {TRUE,0,4,{1017,1033,1043,1468,1051,1172,1173,1174,1180,1189,1038}};
static struct { EVENT_TRACE_PROPERTIES properties; wchar_t name[80]; } trace;
static TRACEHANDLE session, consumer;
static HANDLE finished;
static DWORD own_pid;
static unsigned int captured, rejected;
static int transport_result = 2;
static unsigned int excluded_detail, owner_errors, address_errors;
static UINT64 owned_tcb[2];
static HANDLE connected;
static ULONG observation_error;
static void *payloads[10];
static unsigned int payload_count;
static EVENT_FILTER_DESCRIPTOR filters[2];

static ULONG property(EVENT_RECORD *event, const wchar_t *name, void *value, ULONG size) {
    PROPERTY_DATA_DESCRIPTOR descriptor = {(ULONGLONG)(uintptr_t)name, ULONG_MAX, 0};
    ULONG actual = 0;
    ULONG result = TdhGetPropertySize(event, 0, NULL, 1, &descriptor, &actual);
    if (result != ERROR_SUCCESS) return result;
    if (actual != size) return ERROR_INVALID_DATA;
    return TdhGetProperty(event, 0, NULL, 1, &descriptor, size, value);
}

static void WINAPI observed(EVENT_RECORD *event) {
    if (!IsEqualGUID(&event->EventHeader.ProviderId, &tcpip)) return;
    DWORD owner = 0, status = UINT32_MAX, inspect = UINT32_MAX, reason = UINT32_MAX;
    UINT64 tcb = 0;
    struct sockaddr_in local = {0}, remote = {0};
    USHORT id = event->EventHeader.EventDescriptor.Id;
    if (property(event, L"Tcb", &tcb, sizeof(tcb)) != ERROR_SUCCESS) { ++rejected; return; }
    int detail = id == 1038 || id == 1051 || (id >= 1172 && id <= 1189);
    if (detail) {
        if (!tcb || (tcb != owned_tcb[0] && tcb != owned_tcb[1])) { ++excluded_detail; return; }
        owner = own_pid; // Ownership established from the earlier connection event, not the execution PID.
    } else {
        if (property(event, L"ProcessId", &owner, sizeof(owner)) != ERROR_SUCCESS || owner != own_pid) {
            printf("rejected_owner id=%hu version=%u owner_is_zero=%d\n", id, event->EventHeader.EventDescriptor.Version, owner == 0);
            ++owner_errors; ++rejected; return;
        }
        if (property(event, L"LocalAddress", &local, sizeof(local)) != ERROR_SUCCESS ||
        property(event, L"RemoteAddress", &remote, sizeof(remote)) != ERROR_SUCCESS ||
        local.sin_family != AF_INET || remote.sin_family != AF_INET ||
        local.sin_addr.s_addr != htonl(INADDR_LOOPBACK) || remote.sin_addr.s_addr != htonl(INADDR_LOOPBACK)) {
            ++address_errors; ++rejected; return;
        }
        if ((id == 1017 || id == 1033) && tcb) {
            owned_tcb[id == 1017 ? 0 : 1] = tcb;
            if (owned_tcb[0] && owned_tcb[1]) SetEvent(connected);
        }
    }
    if (captured >= 512) { ++rejected; return; }
    ++captured;
    (void)property(event, L"Status", &status, sizeof(status));
    (void)property(event, L"Inspect", &inspect, sizeof(inspect));
    (void)property(event, L"Reason", &reason, sizeof(reason));
    char line[4096];
    int length = snprintf(line, sizeof(line), "etw id=%hu version=%u qpc=%lld execution_pid=%lu owner_pid=%lu local_port=%hu remote_port=%hu status=%08lx inspect=%lu reason=%lu tcb=%llx stack=",
        event->EventHeader.EventDescriptor.Id, event->EventHeader.EventDescriptor.Version,
        (long long)event->EventHeader.TimeStamp.QuadPart, (unsigned long)event->EventHeader.ProcessId,
        (unsigned long)owner, ntohs(local.sin_port), ntohs(remote.sin_port), (unsigned long)status,
        (unsigned long)inspect, (unsigned long)reason, (unsigned long long)tcb);
    if (detail) {
        const wchar_t *names[] = {L"OldState",L"NewState",L"RequestFlags",L"RequestStatus",L"OldDeliveryState",L"NewDeliveryState"};
        const char *labels[] = {"old_state","new_state","request_flags","request_status","old_delivery","new_delivery"};
        for (int i = 0; i < 6; ++i) {
            DWORD value = 0;
            if (property(event, names[i], &value, sizeof(value)) == ERROR_SUCCESS)
                length += snprintf(line+length, sizeof(line)-(size_t)length, "%s=%lu,", labels[i], value);
        }
    }
    for (USHORT i = 0; i < event->ExtendedDataCount; ++i) {
        EVENT_HEADER_EXTENDED_DATA_ITEM *item = &event->ExtendedData[i];
        if (item->ExtType != EVENT_HEADER_EXT_TYPE_STACK_TRACE64 || item->DataSize < 8) continue;
        const ULONG64 *addresses = (const ULONG64 *)(uintptr_t)item->DataPtr;
        for (size_t j = 1; j < item->DataSize / sizeof(ULONG64) && j <= 64; ++j) {
            if ((size_t)length + 18 >= sizeof(line)) { ++rejected; return; }
            length += snprintf(line+length, sizeof(line)-(size_t)length, "%llx,", (unsigned long long)addresses[j]);
        }
    }
    printf("%s\n", line);
}

static ULONG enable(void) {
    filters[1].Size = offsetof(typeof(event_ids), Events) + event_ids.Count * sizeof(USHORT);
    ENABLE_TRACE_PARAMETERS parameters = {0};
    parameters.Version = ENABLE_TRACE_PARAMETERS_VERSION_2;
    parameters.EnableProperty = EVENT_ENABLE_PROPERTY_STACK_TRACE;
    parameters.EnableFilterDesc = filters;
    parameters.FilterDescCount = 2;
    return EnableTraceEx2(session, &tcpip, EVENT_CONTROL_CODE_ENABLE_PROVIDER, 5, UINT64_MAX, 0, 1000, &parameters);
}

static int before_shutdown(void) {
    observation_error = ControlTraceW(session, trace.name, &trace.properties, EVENT_TRACE_CONTROL_FLUSH);
    if (observation_error != ERROR_SUCCESS) return 0;
    if (WaitForSingleObject(connected, 3000) != WAIT_OBJECT_0) { observation_error = ERROR_TIMEOUT; return 0; }
    event_ids.Count = 11;
    observation_error = enable();
    printf("detail_scope=two_owned_tcbs numeric_metadata_only=1 enable_error=%lu\n", observation_error);
    return observation_error == ERROR_SUCCESS;
}

static DWORD WINAPI consume(void *unused) {
    (void)unused;
    return ProcessTrace(&consumer, 1, NULL, NULL);
}

static DWORD WINAPI watchdog(void *unused) {
    (void)unused;
    if (WaitForSingleObject(finished, 15000) == WAIT_TIMEOUT) {
        ULONG stopped = ControlTraceW(session, trace.name, &trace.properties, EVENT_TRACE_CONTROL_STOP);
        printf("watchdog_stop=%lu\n", (unsigned long)stopped);
        ExitProcess(2);
    }
    return 0;
}

int main(int argc, char **argv) {
    if (argc != 2 || (strcmp(argv[1], "--check") && strcmp(argv[1], "--capture") && strcmp(argv[1], "--capture-data"))) return 2;
    own_pid = GetCurrentProcessId();
    wchar_t pid_value[20];
    swprintf(pid_value, 20, L"%lu", (unsigned long)own_pid);
    PAYLOAD_FILTER_PREDICATE predicate = {L"ProcessId", 0, pid_value};
    ULONG error = ERROR_SUCCESS;
    for (int i = 0; i < 4 && error == ERROR_SUCCESS; ++i) {
        for (int version = 0; version <= (i >= 2 ? 2 : 1); ++version) {
            EVENT_DESCRIPTOR descriptor = {0};
            descriptor.Id = event_ids.Events[i]; descriptor.Version = (UCHAR)version;
            error = TdhCreatePayloadFilter(&tcpip, &descriptor, FALSE, 1, &predicate, &payloads[payload_count]);
            if (error != ERROR_SUCCESS) break;
            ++payload_count;
        }
    }
    if (error == ERROR_SUCCESS) error = TdhAggregatePayloadFilters(payload_count, payloads, NULL, &filters[0]);
    filters[1].Ptr = (ULONGLONG)(uintptr_t)&event_ids;
    filters[1].Size = sizeof(event_ids);
    filters[1].Type = 0x80000200; // EVENT_FILTER_TYPE_EVENT_ID
    if (error != ERROR_SUCCESS || !strcmp(argv[1], "--check")) {
        printf("payload_filters=%u process_scope=self event_ids=4 setup_error=%lu capture_started=0\n", payload_count, (unsigned long)error);
        goto free_filters;
    }
    WSADATA data;
    if (WSAStartup(MAKEWORD(2,2), &data) != 0) { error = ERROR_INVALID_FUNCTION; goto free_filters; }
    (void)wfp_metadata; // Metadata is collected separately by socket-shutdown-path.exe.
    trace.properties.Wnode.BufferSize = sizeof(trace);
    trace.properties.Wnode.Flags = WNODE_FLAG_TRACED_GUID;
    trace.properties.Wnode.ClientContext = 1;
    trace.properties.LoggerNameOffset = offsetof(typeof(trace), name);
    trace.properties.BufferSize = 64;
    trace.properties.MinimumBuffers = 2;
    trace.properties.MaximumBuffers = 8;
    trace.properties.FlushTimer = 1;
    trace.properties.LogFileMode = EVENT_TRACE_REAL_TIME_MODE | EVENT_TRACE_NO_PER_PROCESSOR_BUFFERING;
    swprintf(trace.name, 80, L"ClauductSocketProbe-%lu", (unsigned long)own_pid);
    error = StartTraceW(&session, trace.name, &trace.properties);
    if (error != ERROR_SUCCESS) { WSACleanup(); goto free_filters; }
    HANDLE reader = NULL, timer = NULL;
    finished = CreateEventW(NULL, TRUE, FALSE, NULL);
    if (!finished) { error = GetLastError(); goto stop; }
    connected = CreateEventW(NULL, TRUE, FALSE, NULL);
    if (!connected) { error = GetLastError(); goto stop; }
    timer = CreateThread(NULL, 0, watchdog, NULL, 0, NULL);
    if (!timer) { error = GetLastError(); goto stop; }
    EVENT_TRACE_LOGFILEW logfile = {0};
    logfile.LoggerName = trace.name;
    logfile.ProcessTraceMode = PROCESS_TRACE_MODE_REAL_TIME | PROCESS_TRACE_MODE_EVENT_RECORD | PROCESS_TRACE_MODE_RAW_TIMESTAMP;
    logfile.EventRecordCallback = observed;
    consumer = OpenTraceW(&logfile);
    if (consumer == (TRACEHANDLE)INVALID_HANDLE_VALUE) { error = GetLastError(); goto stop; }
    reader = CreateThread(NULL, 0, consume, NULL, 0, NULL);
    if (!reader) { error = GetLastError(); CloseTrace(consumer); goto stop; }
    error = enable();
    if (error == ERROR_SUCCESS) {
        LARGE_INTEGER frequency;
        QueryPerformanceFrequency(&frequency);
        printf("capture_started=1 pid=%lu max_seconds=15 max_buffers_kib=512 disk_log=0 qpc_frequency=%lld\n",
            (unsigned long)own_pid, (long long)frequency.QuadPart);
        observe_before_shutdown = before_shutdown;
        int with_data = !strcmp(argv[1], "--capture-data");
        transport_result = trial(0, with_data ? 6 : 0, with_data);
        printf("transport_result=%s\n", transport_result == 2 ? "NOT_RUN_SETUP_ERROR" : transport_result ? "FAIL" : "PASS");
        error = EnableTraceEx2(session, &tcpip, EVENT_CONTROL_CODE_DISABLE_PROVIDER, 0, 0, 0, 1000, NULL);
        if (observation_error != ERROR_SUCCESS) error = observation_error;
        if (transport_result == 2 && error == ERROR_SUCCESS) error = ERROR_INVALID_DATA;
    }
stop:;
    ULONG stopped = ControlTraceW(session, trace.name, &trace.properties, EVENT_TRACE_CONTROL_STOP);
    if (reader) {
        if (WaitForSingleObject(reader, 3000) != WAIT_OBJECT_0) {
            CloseTrace(consumer);
            if (WaitForSingleObject(reader, 1000) != WAIT_OBJECT_0) ExitProcess(2);
            error = ERROR_TIMEOUT;
        } else {
            DWORD reader_result = ERROR_SUCCESS;
            if (!GetExitCodeThread(reader, &reader_result)) error = GetLastError();
            else if (reader_result != ERROR_SUCCESS) error = reader_result;
            CloseTrace(consumer);
        }
        CloseHandle(reader);
    }
    if (finished) SetEvent(finished);
    if (timer) { WaitForSingleObject(timer, 2000); CloseHandle(timer); }
    if (finished) CloseHandle(finished);
    if (connected) CloseHandle(connected);
    WSACleanup();
    printf("capture_error=%lu stop_error=%lu captured=%u rejected=%u lost=%lu\n", (unsigned long)error,
           (unsigned long)stopped, captured, rejected, (unsigned long)trace.properties.EventsLost);
    printf("excluded_nonmatching_detail=%u owner_errors=%u address_errors=%u\n", excluded_detail, owner_errors, address_errors);
    printf("buffers_used=%lu buffer_kib=%lu\n", trace.properties.NumberOfBuffers, trace.properties.BufferSize);
    if (error == ERROR_SUCCESS && (!owned_tcb[0] || !owned_tcb[1] || !captured || rejected || stopped != ERROR_SUCCESS ||
        trace.properties.EventsLost || trace.properties.NumberOfBuffers > 8)) error = ERROR_INVALID_DATA;
free_filters:
    for (unsigned int i = 0; i < payload_count; ++i) TdhDeletePayloadFilter(&payloads[i]);
    if (filters[0].Ptr) TdhCleanupPayloadEventFilterDescriptor(&filters[0]);
    printf("collector_error=%lu\n", (unsigned long)error);
    return error != ERROR_SUCCESS ? 2 : !strcmp(argv[1], "--check") ? 0 : transport_result;
}
