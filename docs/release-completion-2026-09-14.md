# 변경된 출하 목표와 작업 판정

2026-09-14 사용자가 정상 인증 갱신·기본 압축 실호출·장기 시험을 제외하고 남은 작업의 구현과 로컬 출하를 요청했다. 목표는 약속된 비-Anthropic 서버 전용 기능과 제한된 실제 개발·복구를 완성하고 동일 최종 후보의 증거로 로컬 출하 PASS를 판정하는 것이다. 외부 게시·배포는 포함하지 않는다.

정상 갱신 판단은 사용자 Codex CLI 로그인 기준, 기본 400K/320K 압축 발동은 사용자 실사용 검증, 4h/24h×3/72h 단계는 범위밖이다. 기존 설정·변환·라우팅·오류·권한 검사는 유지하며, 제외를 PASS로 기록하지 않는다. 원래 명세와 F01~F23 중 제외된 부분 이외의 필수 요구는 유지한다.

기준 후보는 `6c728afa39bd5d5166dd82ba981d6b366fe03580`, 구현 위치는 `.tmp/unattended-release/implementation`, 전용 branch는 `work/unattended-release-2026-09-13`이다. 사용자 루트의 README +2/-0와 기존 untracked 상태를 보존한다. 수정은 이해한 작업 diff만으로 되돌릴 수 있게 분리한다.

| 순서 | 남은 범위 | 현재 상태 | 필요한 증거 |
|---|---|---|---|
| 1 | gateway·HTTP 종료·취소·동시 실행 | 반닫기 FAIL 보존 / OS 원인 추적 보류 | CMD에서도 동일 EOF. 환경 한계로 기록하고 독립 작업 진행; 관련 HTTP 회귀 전체의 원인이 확정된 것은 아님 |
| 2 | 부모·자식 알림/실패/취소, Workflow 적용 가능한 계약 | 자식 취소 local-native 4사례 PASS / 나머지 미완료 | 실제 backend 결합과 직접 부모 알림·Workflow 미완료는 별도 유지 |
| 3 | 실제 개발·소유/사용량·효과 대조·파일 적용·복구 | 부분 검증 | 현재 실제 경로의 좁은 개발·독립 oracle·중복/유실/거짓 완료 0 |
| 4 | 남은 설정·도구·권한·변조·기록 경계 | 부분 검증/특정 차단 | 요구별 정상·거부 증거. 기존 guard 거부는 우회하지 않음 |
| 5 | 동일 최종 후보 회귀·실제 실행·로컬 ZIP | 미완료 | 관련/전체 회귀, 필요한 실제 luna/max·sol/low, 재현 ZIP/hash/새 경로/회수 |

장기 시험 전용 반복과 관리기 확장을 추가하지 않는다. 기존 통과 증거는 코드 영향과 적용 범위를 대조하여 재사용하고, 새로운 실패·변경·미검증이 필요한 검사만 실행한다. ZIP은 최종 후보가 정해진 뒤 만든다.

첫 조사 한도는 외부 요청 0, credential 읽기 0, 공개 loopback만, 검사당 20초 이하와 전체 로컬 runner 60초 이하다. 기존 실패의 동일 반복·시간 상한 확대는 하지 않는다. 실제 호출은 최신 `live-stream-3903e0a-20260914`의 누적 시도 297~301 및 미관측 예약을 이어받는다. 현재 요청 cap 301/잔여 0이므로 새 실호출 전에 변경된 제한 실행 목록과 기존 측정으로 별도의 유한 예산을 산정·기록해야 하며, 아직 새 요청은 예약하지 않았다. 사용자 비용 축소 요구를 반영하고 과거 소비를 초기화하지 않는다.

구현 영향 회귀는 30초 runner 안에서 수행했고, 새 native 비교는 실행 전 별도 budget.json에 각 30초/16개 공개 응답 상한을 기록했다. 장기 시험으로 확대하지 않았다.

목표 도구의 기존 목표는 blocked이며 새 objective 등록은 `unfinished goal` 때문에 거부됐다. 완료되지 않은 예전 목표를 완료로 변경하지 않았다. 이 문서가 사용자의 최신 범위를 반영한 현재 로컬 목표다. 사용자 재개 요청으로 개발 권한 차단은 해소되었고, 기존 개별 guard 차단은 유지한다. 현재 출하 판정은 HOLD다.

## 재개 후 관측과 구현

- HTTP 종료 비교: 기존 경로 3회 중 2회는 약 1초 fallback, FIN을 보내는 실험 경로는 3회 모두 약 4~13ms였다. 그러나 해당 수정의 실제 기존 검사에서 registration ECONNRESET과 반닫기 응답 유실이 발생했다. 실험 수정을 철회했으며 `src/http-close.mjs`는 기준 코드와 같다. 증거는 `.tmp/release-completion-20260914/close-differential-result.json`, `http-gateway-first-fix.json`이다. 지연 상한이나 assertion을 완화하지 않았다.
- 실패 알림: 기존 구현은 실패 알림을 무조건 거부했다. 새 `failed()`는 현재 검증 요청의 실패만 기록하며 이후 begin/정상 완료/늦은 callback과 구분한다. 부모 재개는 실패 receipt, 직접 자식 관계, 같은 metadata, 새 native API 오류의 ID/role/stop_sequence/시간을 함께 요구한다. 기존 성공 receipt와 섞인 batch를 원자적으로 소비하며 재사용·위조·동시 재개를 거부한다. 취소·사용자 중단은 실패 성공으로 대체하지 않는다.
- 최초 실패 baseline은 `failed-completion-baseline.json`의 NOTIFICATION_HEADER다. 최종 `failed-completion-final-regression.json`은5파일 PASS/12616.9509ms이며 새23개, 기존 completion46/batch11/diagnostics69 및 agent-selection 검사를 포함한다. 내부 symlink NOT_RUN은 별도로 유지한다. 실제 HTTP handler의 실패 receipt 연결과 cleanup도 검사했다.
- 실제 native 비교는 외부 transport 없이 고정 공개 응답만 사용했다. 처음 두 sol fixture는 실행 도구 목록에 Read가 없어 자식이 시작되지 못했고, 첫 fixture의 main 조기 오류도 보존했다. 구성 오류를 수정한 sol root `native-failure-44ac0e0036f24efba77b3bfa42f6b826`과 luna root `native-failure-394d0c8e9e9d45488f28f734fb809ead`에서 각각 자식 API 실패가 발생했다. sol은7개 로컬 요청/2084ms, luna는2122ms, 두 실행 모두 회수 잔여0이다. 부모 직접 알림/재개는 실패로 남았다. native 오류 record의 관측 형식은 `native-failure-record-shape.json`에 값 없는 구조로 남겼다.
- `.tmp/release-completion-20260914` 아래 증거와 새 native fixture만 추가했다. 개인 profile·기존 세션 원문·인증 값을 입력으로 복제하지 않았고 실제 backend 추가 요청0이다. 단위 검사, native+공개 응답, 실제 backend를 구분한다.

## 확인한 native 경계와 후속 범위

[공식 subagent 문서](https://code.claude.com/docs/en/sub-agents#resume-subagents)는 SendMessage 재개의 결과를 중간 부모에게 돌려주는 설명을 interactive 세션에 한정한다. 이번 `-p` 실행에서는 부모 transcript에 알림이 없었고 메인으로 전달되는 것을 직접 관측했다. 문서만으로 모든 버전의 비대화형 동작을 단정하지 않는다. [공식 저장소의 보고 #81438](https://github.com/anthropics/claude-code/issues/81438)에도 중첩 알림의 다른 버전/상황에서의 전달 문제가 있지만, 이는 이번 원인 확정이나 제품 수정 완료의 증거는 아니다.

[공식 Workflow 문서](https://code.claude.com/docs/en/workflows#resume-after-a-pause)는 같은 세션에서 저장 결과를 재사용하고, 실패 지점과 그 이후를 재실행하며, 저장 결과 자체가 없으면 `nothing to resume`로 종료한다고 설명한다. 따라서 cache miss 용어를 저장 결과 전체 소실과 개별 agent 재실행으로 구분할 필요가 있다. 기존 scriptPath 거부 경로를 새 문서만으로 재시도하지 않았다. Workflow의 현재 미완료는 아직 닫지 않았다.

`CLI_VERSION_UNVERIFIED`는 `docs/audit-2026-09-10-cli-version-policy.md`와 실행 코드에 명시된 비차단 진단이다. 이 문자열 자체를 출하 실패로 세던 브리핑 해석은 정정하며, 실제 호환성 검증 범위는 그대로 필요하다.

사용자 일반 PowerShell 비교 결과가 도착했다. IPv4·IPv6의 sendFin=false는 각각113/109ms 뒤1바이트를 읽었고, sendFin=true는 각각0/1ms에0바이트 EOF를 받았다. 서버는 네 경우 모두1바이트를 읽고 썼으며 오류가 없었다. 사용자 원결과는 `.tmp/release-completion-20260914/user-tcp-comparison.json`에 출처와 함께 보존한다. Clauduct·Node·에이전트 터미널에만 국한된 현상이라는 가설은 배제하지만 Windows 구성요소·필터·런타임 중 원인은 확정하지 않는다. 인증·전역 설정 변경·기존 프로세스 종료는 실행하지 않았다. 나머지 기능·실제 개발 경로·기존 실제 원장 registry 차단·최종 후보 검증도 미완료다.

실패 receipt 구현 checkpoint는 `e189590bef6131e33e12cd89b6e108c7d89fc786`이다. 후속 공개 IPv4 loopback 비교에서 NetworkStream/Socket 비동기/Socket 동기 읽기 모두 sendFin=false는 정상1바이트를 읽었고, sendFin=true는3/4/6ms에0바이트를 읽었다. 첫 두 반닫기 사례는 뒤이은 서버 쓰기도 IOException이었고, 동기 Socket 사례는111ms에 쓰기에 성공했다. `.tmp/release-completion-20260914/tcp-read-probe-result.json`에6사례 원결과를 보존한다. [Microsoft의 정상 종료 순서](https://learn.microsoft.com/en-us/windows/win32/winsock/graceful-shutdown-linger-options-and-socket-closure-2)는 SD_SEND 뒤 서버의 남은 응답을 읽는 절차다. 이번 결과를 정상 동작이나 특정 제품의 결함으로 단정하지 않는다.

설치된 Windows PowerShell의 구형 .NET과 비교하려던 `run-legacy-tcp-probe.ps1`은 실행 정책의 UnauthorizedAccess로 시작 전 거부됐다. `legacy-tcp-probe-not-run.json`에 기록하며 실행 정책 변경·inline command·다른 interpreter로 해당 거부 효과를 재현하지 않았다. 진단용 pwsh 프로세스에 로드된 비-Microsoft DLL에서 확인된 것은 ICU/Newtonsoft뿐이지만, 커널 필터나 외부 서비스 부재의 증거는 아니다. TCP 원인 특정은 아직 차단되어 있고 제품 전체가 안전하다는 판정으로 확대하지 않는다.

## 자식 취소와 필수 선택 회귀 — 사용자 TCP 결과 반영 후

`native-agent-cancel-verified.json`은 현재 구현에서 실제 native Agent 두 개를 시작하고, 정확한 Agent 호출의 반환 ID에만 TaskStop을 연결한4사례의 독립 대조다. sol/low·luna/max 각각 alpha/beta를 번갈아 취소했다. native가 실제 TaskStop·TaskOutput을 실행했으며 취소된 요청1개는 CANCELLED로 남고 형제1개와 메인5개 요청은 성공했다. 취소 뒤 주입한 늦은 protocol 이벤트는 CANCELLED로 거부되어 snapshotMismatch가 기록되지 않았고, 형제 결과가 메인에 도착했다. 이는 실제 backend 패킷이나 모델 추론의 실호출 검증은 아니다.

| 조합·취소 대상 | 실행 디렉터리(.tmp 하위) | 경과 | 결과 |
|---|---|---|---|
| sol/low·alpha | native-agent-cancel-845bad89f37b40faabcfd1a07d8bb359 | 1820ms | PASS |
| sol/low·beta | native-agent-cancel-966b8da864bc43e8b04dcc6f97822ebc | 2156ms | PASS |
| luna/max·alpha | native-agent-cancel-5c676eb39fac4c0abe269bdb0002725d | 1905ms | PASS |
| luna/max·beta | native-agent-cancel-f1271e19eca54aec87c4a7474bc7c86c | 1946ms | PASS |

합계28개 고정 공개 응답/7827ms, 각 exit0·정리9항목 true·잔여 프로세스0이다. 최초 `native-agent-cancel-610b33977b2b4a92bce3e45fefaaedfd`는 동작을 완료했지만 fixture transport의 activeSockets/activeRequests 계수 누락으로 cleanup 검증이 실패하여 exit1이었다. 실제 진행 중 요청 계수를 추가한 뒤 위4사례를 실행했으며 최초 실패를 보존했다. 실제 backend 추가 요청과 credential 읽기는 전 과정0이다. 실행 전 각30초/12요청, 자식 대기10초 한도를 고정했다. 개인정보·기존 profile·과거 세션 원문을 fixture로 복제하지 않았다.

선택 변경에 필요한 추가 회귀 `selection-required-regression.json`은 Workflow 선택36개와 admission 기한/취소/압력 회복의2파일 PASS/3759.5433ms다. 기존5파일과 합쳐 해당 선택 변경의 관련7파일이 통과했다. Workflow 내부 native 실행·symlink NOT_RUN은 유지하며 실제 Workflow 재개 성공으로 세지 않는다.

사용자 요청의 셸 비교는 [TCP/셸 판정](tcp-shell-assessment-2026-09-14.md)에 기록했다. 같은 Node·검사·최소 환경에서 PowerShell 직접 실행과 cmd.exe 실행 모두 반닫기4ms/0바이트 EOF, 일반 응답1바이트·회수0이었다. CMD 전환은 해결책이 아니며 Windows 전체 결함 또는 사용자 설정 실수로 단정하지 않는다. 현재 Windows 환경의 미해결 제약으로 기록하고 추가 OS 진단을 보류한 채 Workflow 재개 작업으로 진행한다.

현재 새 ZIP 생성과 목표 완료 처리는 하지 않았다. 차단과 미완료를 다음처럼 구분한다.

- 재현 FAIL: TCP 반닫기/관련 gateway 회귀, native 직접 부모 알림. 프로토콜을 바꾸거나 메인 중계를 넣어 직접 부모 알림 성공으로 표시하지 않는다.
- 특정 효과 BLOCKED: 실제 원장 registry FileSystemWrite, Workflow scriptPath 재개, 동적 링크·worker fsync 등 기존 거부. 구형 PowerShell 검사는 선택적 진단의 추가 거부이며 제품 기능 전체 차단으로 확대하지 않는다.
- 남은 구현·증거: 실제 개발/복구·소유 대조의 적용 범위, 허용된 구성별 기능/권한 경계, 같은 최종 후보의 필요한 실제 실행과 릴리즈. 과거 후보의 PASS와 이번 고정 응답 PASS를 통합 실호출 PASS로 복사하지 않는다.
- 다음 작업 순서: Workflow 재개 계약의 안전한 구현 범위를 기존 native 거부와 분리해 확정하고, 실제 원장 연결 차단이 해소되기 전까지 독립 로컬 회귀를 마무리한다. TCP 비교의 추가 반복이나 실행 정책 변경, 장기 시험, 새 실제 요청 예산 확대는 예약하지 않았다.
