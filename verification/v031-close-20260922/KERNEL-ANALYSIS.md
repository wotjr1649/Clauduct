# 보호 On: 잘못된 수신 FIN 주입 경로

2026-09-22. 사용자 통지대로 AdGuard On을 유지했다. 설정·서비스·driver·설치본 변경은 0건이다.
**조기 EOF의 직접 실행 경로는 확인했다. AdGuard 내부의 결함 명령과 수정 패치는 아직 확정하지 못했다.**
원래 Node 31개·.NET 35개·raw TCP 400개는 기존 FAIL/HOLD를 유지한다.

## 관측으로 확인한 원인

[추가 Winsock 진단](../socket-shutdown-path.c)은 서버/클라이언트 각각에 대해 전송 0 bytes,
6 bytes 미소비, 6 bytes 완전 소비의 여섯 조건을 검사했다. 여섯 조건 모두 송신 방향만 닫은
소켓의 `recv`가 EOF였다. 반대편은 그때까지 `shutdown`과 `closesocket`을 호출하지 않았다.
최종 실행 시간은 323–1,687µs다. [전체 관측](shutdown-path.txt).
따라서 데이터 미소비, 특정 서버 역할, Go/Node/.NET의 종료 구현만으로 이 반례를 설명할 수 없다.

`SO_PROTOCOL_INFOA`의 chain length는 1, provider는 기본 TCP provider,
`SO_LINGER`는 비활성 상태였다. 읽기 전용 WFP 열거에서 AdGuard에 귀속되는 callout 34개를
확인했고, 그중 IPv4 STREAM callout ID는 264·265·266이었다. 등록 사실만으로 실행된 callback을
판정하지 않는다. 다른 STREAM callout도 존재하므로 상호작용 가능성은 열어 둔다.

[ETW 관측기](../socket-shutdown-etw.c)는 재현 프로세스의 연결 이벤트에서 서버·클라이언트
TCB를 먼저 확인한다. 이후 숫자 필드만 있는 상태 전이·FIN 주입 이벤트 중 이 두 TCB에 해당하는
것만 보존한다. 포인터 형식 TCB는 Windows payload filter가 지원하지 않아 이 단계는 consumer의
일치 검사로 제한한다. 호스트의 packet body, 다른 연결 정보, ETL 파일은 기록하지 않는다.
최대 15초·512KiB로 제한했으며 최종 실행의 실제 trace buffer는 64KiB 2개였다.

서버가 보낸 6 bytes를 클라이언트가 모두 읽은 뒤 서버만 `shutdown(SD_SEND)`한 최종 실행:

| 송신 종료 직전부터 경과 | 확인된 동작 |
|---:|---|
| 63.5µs | 서버의 `TcpDisconnectTcbInspectComplete`, `Inspect=1`, status 0 |
| 364.5µs | 서버 TCP 상태 4 → 5, `InetInspectInjectDisconnect → TcpDisconnectTcb` |
| 374.1µs | 같은 서버 TCP 상태 5 → 8, **`InetInspectInjectRemoteDisconnect → TcpInjectFin → TcpAcceptFin`** |
| 383.8µs | 서버 `recv=0`. 상대의 shutdown/close는 여전히 0회 |
| 398.7µs | 서버 `TcpTcbInjectFinComplete` 이벤트, request status 0 |
| 637.7µs | 클라이언트 `TcpTcbInjectFinComplete` 이벤트 |
| 749.8µs | 재현기가 결과·상태를 기록한 후 소켓 정리 시작 |

관측 행의 출력 순서는 ETW 버퍼 전달 순서다. 위 표는 같은 QPC 시계로 정렬했다.
`recv` 자체의 대기는 364µs이며, 송신 종료 호출 직전부터의 383.8µs와 다른 구간이다.
숫자 상태는 TCPIP ETW의 값이며 `GetExtendedTcpTable`의 MIB 상태 번호와 섞지 않는다.
FIN 완료 이벤트는 처리 후 기록되므로 EOF 반환보다 약간 늦을 수 있다. **EOF 직전에 실행된
서버의 `TcpAcceptFin` 상태 전이가 반대 방향 FIN의 직접 증거다.**

[정규화한 기계 기록](kernel-trace.json)은 이벤트 17개, rejected 0, lost 0,
collector error 0, session stop error 0이다. 전송 판정은 FAIL이며 실행 exit code도 1이다.
계측 성공을 전송 PASS로 바꾸지 않았다. 절대 커널 주소·TCB 주소는 보존하지 않고
확인한 모듈 범위의 RVA와 서버/클라이언트 역할로 바꿨다.

## 실행된 함수와 외부 구성요소의 확인 지점

Microsoft의 해당 바이너리와 일치하는 공개 PDB를 SDK `DbgHelp`로 로드하여 아래 경로를 확인했다.
`SymGetModuleInfo64`의 `SymType=SymPdb`, `PdbUnmatched=false`를 확인했다.
`tcpip.sys`는 `10.0.26100.8521`, `NETIO.SYS`와 `fwpkclnt.sys`는 `10.0.26100.8870`이다.
심볼 캐시는 작업 폴더 `.tmp/symbol-cache`에만 두었고 전역 심볼 설정은 바꾸지 않았다.
공개 심볼 조회 외에 backend 호출은 없었다.

```text
NETIO!StreamPermitRemoveDataWorkerRoutine+0x14a
  NETIO!StreamPermitDataHelper+0x68
    NETIO!StreamInjectReceiveToStack+0xf7
      NETIO!WfpInetInspectInjectRemoteDisconnect+0x17
        tcpip!InetInspectInjectRemoteDisconnect+0xc
          tcpip!TcpInjectFin+0x90
            tcpip!TcpAllowFin+0x1f
              tcpip!TcpAcceptFin+0x27e
```

이 경로는 송신 종료한 서버에 대한 수신 FIN 처리다. 같은 연결의 앞선 검사 완료는
`FwppDiscardClonedStreamDataWorkItemRoutine → FwpsFreeCloneNetBufferList0 →
StreamRequestNetBufferListCompletionFn → InetInspectDisconnectComplete →
TcpDisconnectTcbInspectComplete` 경로였다. 이 Windows 함수들이 결함의 소유자라는 뜻은 아니다.
비동기 worker stack에는 요청을 큐에 넣은 AdGuard 함수가 남지 않았다.

설치된 `adgnetworkwfpdrv.sys 8.0.91.0`을 실행·수정하지 않고 정적으로 분석했다.
[파일 hash와 API 연결 근거](driver-call-sites.txt)에 해당하는 **구체적인 후속 계측 위치**는 다음과 같다.

| 이미지 RVA | 역할 | 확인해야 할 값 |
|---|---|---|
| 함수 `0xB350`, 호출 `0xB529`·`0xB606` | 일반 스트림 큐에서 `FwpsStreamInjectAsync0` 호출 | 요청 구조체 `+0x20`의 `streamFlags`, flowId, calloutId, layerId, dataLength, 반환값, 큐 생산자 |
| 함수 `0x10250`, 호출 `0x1027B` | `FwpsDiscardClonedStreamData0` 호출 | 복제한 종료 indication의 소유권·해제 시점과 대응하는 주입 완료 |
| 함수 `0xFE20`, 호출 `0xFECC` | 별도 연결 목록을 순회하며 상수 flags `0x90000`으로 주입 | 이번 반종료에서 이 별도 경로가 실행되는지부터 확인 |

RVA는 이 파일 hash에만 해당한다. 정적 분석만으로 어느 호출이 이번 FIN을 만들었는지,
그 입력이 Core/NetLib의 정책에서 왔는지 driver의 방향 처리에서 왔는지는 확정할 수 없다.
따라서 이 주소에 임의 바이트 패치를 하거나 DLL·driver를 교체할 근거는 없다.

수정이 보존해야 하는 조건은 명확하다. **한쪽의 SEND_DISCONNECT를 처리했다는 이유만으로
같은 소켓의 RECEIVE_DISCONNECT를 생성해서는 안 된다.** 상대가 실제 종료하기 전에는 수신을
유지하고, 큐의 데이터·종료 indication·복제 버퍼 완료를 방향별로 처리해야 한다.
Microsoft의 [스트림 주입 계약](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/fwpsk/nf-fwpsk-fwpsstreaminjectasync0)은
송신·수신 방향과 FIN 플래그를 각각 구분한다. 실제 전달된 `streamFlags`는 이번 ETW에 없으므로
특정 비트가 뒤바뀌었다는 가설을 관측 사실로 기록하지 않는다.

## 검증 범위와 남은 간격

- C 진단 두 파일은 `-Wall -Wextra -Werror` 빌드 PASS. ETW `--check`의 payload filter 10개 구성 PASS.
- 첫 계측에서 종료 이벤트 6개를 거부했던 원인은 `TcpCloseTcbRequest`의 payload 소유자/주소를
  연결 이벤트와 동일하게 요구한 관측기였다. 별도 caller 관측에서 이미 식별한 TCB인데 payload
  소유자가 비어 있음을 확인했고, 최종 관측기는 확인된 TCB로 귀속시킨다.
- 추가 pointer payload filter 구성은 `ERROR_INVALID_PARAMETER(87)`로 실패했다.
  [공식 지원 형식](https://learn.microsoft.com/en-us/windows/win32/api/tdh/ns-tdh-payload_filter_predicate)에
  맞춰 해당 실패 경로는 제거했다. 최종 숫자 이벤트의 보존 범위는 위에 명시했다.
- 원래 세 검사와 실제 TUI는 이번에 다시 실행하지 않았다. 보호·구성요소·제품의 동작 수정이
  없는 상태에서 이전 실패/성공을 새로운 실행 결과로 대체하지 않는다.

다음 근본 수정에 필요한 것은 위 주입 지점의 **합성 연결에 한정된 런타임 인자**와 큐 생성 경로다.
외부 소스 또는 그 위치를 계측한 공식 진단 빌드로 SEND/RECEIVE 매핑과 종료 소유권을 확인하면
driver 결함인지 상위 NetLib/Core 명령인지 판별할 수 있다. 이 단계는 아직 미검증이다.
그 수정 후 직접 Winsock의 대조·여섯 조건, 원래 Node 31개·.NET 35개·raw TCP 400개,
실제 native TUI를 보호 On에서 다시 통과해야 전체 Success다.
이번 추가 자료는 로컬에만 보존했으며 이미 게시된 이슈 본문은 변경하지 않았다.
