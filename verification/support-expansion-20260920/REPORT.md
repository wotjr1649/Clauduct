# S48 지원 확대와 필터 검수 — 2026-09-20

## 판정

설정 병합, Workflow 도구 allowlist·명시적 모델의 커스텀 역할·native 역할의 회차 제한,
빈 Workflow 종료 결과 통지, HTTP non-streaming JSON 응답을 구현했다.
아래 범위의 실제 TUI/API 검증이 있으며, 모든 기능·모든 환경의 무결점 또는 v0.3.0 전체
출시 합격 판정은 아니다. 설치된 release는 변경하지 않았다.

원본 기준은 `7edeb73`(이전 제품 `442c366`). 분리 worktree/branch에서 변경했다.
native `2.1.278`, Windows amd64, Go `1.27.1`, 제품 `CGO_ENABLED=0`이다.
공개 테스트 데이터와 이미 승인된 격리 project/profile만 사용했다.
AdGuard는 사용자 보고상 활성 상태였고 이번 작업에서 설정을 변경하지 않았다.

## 바뀐 동작과 근거

| 대상 | 구현과 확인 범위 |
|---|---|
| `--settings` | inline JSON 또는 명시한 일반 파일을 최대 2 MiB로 읽어 필수 설정과 한 번 병합. 마지막 옵션 우선, duplicate key 거부, 큰 정수 보존, 사용자 hook 유지. 필수 연결/정책/hook을 무력화하는 충돌은 오류 |
| `--setting-sources` | native로 전달. 실제 native CLI + 공개 고정 backend로 user/project/빈 source가 각각 지정 effort를 적용함을 확인 |
| Workflow `tools` | raw agent/독립 plan step에 최대 64개 정확 도구 이름 또는 `[]`. native catalog와 callable set을 함께 제한. 과거 tool result가 새 도구 권한으로 되살아나지 않음. role/permission을 확장하지 않음 |
| custom `agentType` | explicit model 필수. native 역할 prompt/tools/maxTurns 유지, backend의 확정 model/effort와 native turn을 대조. 역할 이름을 workflow 유형 판정으로 잘못 사용하던 경로를 request class와 선택 출처로 수정 |
| `maxTurns` | 직접 Workflow 옵션은 native VM이 적용하지 않아 거부 유지. native 역할 정의의 `maxTurns:1`로 Read 1회 후 종료를 실제 native CLI와 TUI에서 확인. HTTP 요청 수를 회차 제한 구현으로 사용하지 않음 |
| 빈 Workflow 결과 | native 종료 + 검증한 journal의 `result:""`는 종료 증거다. 쓸 수 있는 본문이 없으면 `result_unavailable`→`unavailable_reported`로 전달. 기존에는 미완료 journal과 혼동해 기다리던 실제 결함을 수정 |
| non-streaming API | `stream:false` 또는 생략 시 기존 검증된 SSE 변환 결과만 JSON으로 조립. upstream 생성 1회, 16 MiB/1,024 block 상한, 완료 전 부분 응답/도구 미반환. 잘못된 stream 타입은 계속 거부. native 자동 fallback 재생성은 계속 비활성 |

JSON 응답은 text, tool_use의 큰 정수, opaque reasoning 기록, hosted search result 연결,
usage와 stop_reason을 보존한다. 잘린 upstream 응답은 성공 JSON으로 반환하지 않는다.
Messages API의 stream 생략 의미는 [Anthropic 공식 SDK](https://github.com/anthropics/anthropic-sdk-python/blob/main/src/anthropic/types/message_create_params.py)의 non-streaming 선언과 대조했다.
세션 인증·요청 class·컨텍스트 정책 등 기존 gateway 실행 조건은 그대로 적용한다.

## 실제 TUI/API

자료의 상세 수치·검사 파일 해시는 [evidence.json](evidence.json)에 보존한다.

- `c47b5cc3-ff07-48e5-a261-01b20c348126`: 실제 구독 backend/native TUI.
  커스텀 역할 자식은 `Read` 한 번, 정상 tool result, 최종 text 없음으로 종료했다.
  부모가 완료 이벤트와 결과 미확보 통지를 받아 `S48_PARENT_RECOVERED`로 답했다.
  다음 같은 세션에서 두 Workflow 자식을 병렬 실행했다. Terra는 Read 1회 후
  `S48_READ_OK`, Luna는 도구 없이 `43 S48_NO_TOOLS_OK`를 반환했다.
  부모 결과 전달·후속 대화가 끝났다. API 오류 0, native 도구 오류 0, 자식 재실행 없음.
  `unacquiredResultsInRecent:1`은 의도적으로 빈 결과를 만든 시험의 정직한 잔여 표시다.
- `1e7aaaea-2bd2-44c5-b7ba-33de61f576d8`: 실제 native TUI + 고정 공개 backend.
  구독 backend 시험과 구분한다. 프록시가 native의 HTTP 연결 종료를 gateway로 전달하지
  않도록 결함을 주입했다. 부분 도구 인자 242개 생성 중 Esc 후
  `cancellationSource:native_abort_receipt`로 중단, 같은 세션의 `PUBLIC_AFTER_ESC_OK` 응답 확인.
  `must-not-exist-s47.txt`는 생성되지 않았다. 클라이언트 연결 종료와 upstream 취소 사이
  관측 간격 535ms. 실제 UI 정상 종료, watchdog 강제 종료 없음.
- `TestRuntimeEvidenceNonStreaming`: 4개 모델 각 1회의 공개 live HTTP 요청.
  Astra/Terra `stream:false`, Sol/Luna stream 생략. 모두 HTTP 200 JSON,
  `PUBLIC_NONSTREAM_OK`, input 29/output 8, `end_turn` 확인. 임의 클라이언트와 모든
  multimodal non-streaming 조합을 전수 검증했다는 뜻은 아니다.

## 생성 파라미터의 실제 한계

별도 API 키나 외부 계수 서비스를 추가하지 않았다. 현재 구독 backend에
4개 모델 × 5조건 = 20개의 고정 공개 요청을 보냈다.

| 조건 | 결과 |
|---|---|
| baseline | 4/4 HTTP 200, completed |
| `temperature:0.2` | 4/4 HTTP 400 |
| `top_p:0.8` | 4/4 HTTP 400 |
| `max_output_tokens:32` | 4/4 HTTP 400 |
| `stop:["PUBLIC_STOP"]` | 4/4 HTTP 400 |

`stop` 실측을 `stop_sequences`라는 정확한 필드의 실측으로 부르지 않는다.
[일반 Responses API 문서](https://developers.openai.com/api/reference/cli/resources/responses/methods/create)에
필드가 있어도 구독 경로에서 적용된다는 근거가 아니다. 출력 잘라내기는 서버 생성 상한이나
sampling 제어와 같지 않으므로 지원으로 표시하지 않았다. 미래 backend 변경 가능성은 열려 있다.

## 필터 문제: 개선 가능한 부분과 보장 불가능한 부분

이번에 재확인한 개선은 **HTTP 취소 통지가 유실돼도 검증된 native 취소 영수증으로
해당 요청을 회수하고 다음 요청을 처리하는 것**이다. 이 제어는 이전 S47 제품 구현을
재사용했다. 응답 생성 재시도나 방화벽/필터 해제, 다른 목적지로의 우회는 추가하지 않았다.

필터가 요청 또는 응답의 모든 가능한 경로를 계속 차단하면 원격 전달 성공을 보장할 수 없다.
로컬 write/flush의 성공은 원격 애플리케이션의 수신 확인이 아니다. 서버가 이미 실행했으나
응답만 차단된 경우에는 무조건 재송신하면 중복 효과가 생길 수 있다. 이 경우 성공을 만들어
기록하거나 timeout을 없애는 것은 해결이 아니다.

- TCP keepalive/timeout은 끊김 감지와 자원 회수에 도움을 주지만 차단된 패킷을 전달하지 못한다.
- `SetLinger(1)`은 socket close의 대기/종료 방식을 바꾸는 시험이지 필터 통과 보장이 아니다.
  이번 제품에 추가하지 않았다. [Microsoft Winsock 설명](https://learn.microsoft.com/en-us/windows/win32/winsock/graceful-shutdown-linger-options-and-socket-closure-2),
  [TCP timeout 규약](https://www.rfc-editor.org/rfc/rfc9293.html#section-3.10.8).
- AdGuard의 SockFilter/WFP, localhost filtering, redirect 설정은 서로 다른 간섭 지점이다.
  변경 없는 조건에서는 수신/취소/종료를 분리해 관측해야 한다. 특정 버전/드라이버 조합의
  추가 호환 수정 가능성을 부정하지 않지만 임의 필터 전체로 일반화하지 않는다.
  [AdGuard network settings](https://kb.adguard.com/kb/adguard-for-windows/settings/app-settings/network-settings/),
  [advanced settings](https://adguard.com/kb/adguard-for-windows/settings/app-settings/advanced-settings/).
- 다른 포트·TLS·로컬 pipe도 별도 경로가 허용될 때의 대안이다. 모든 경로가 막혔다는 전제의
  답이 아니며, native HTTP 인터페이스를 임의 교체해 완전 호환이라고 할 수 없다.
- 필터가 정상화된 뒤의 새 요청 성공과 차단 중의 전달 성공은 다른 조건이다. 후자를 구현했다고
  주장하지 않는다. 원격 backend가 실제 계산을 멈춘 시각까지 이번 고정 fixture가 증명하지 않는다.

## 실패와 검수

실패한 시도를 지우거나 재실행 성공으로 덮지 않았다. raw 로그는 이 검증 디렉터리에
보존하고, 공개 가능한 요약/해시만 evidence.json에 집계한다.

1. 초기 도구 allowlist 테스트의 `strings.Replace` 인자 누락, non-streaming `Message` 타입명
   충돌 및 잘못된 text builder 호출은 compile 실패였다. 수정 후 해당 회귀 통과.
2. 커스텀 역할은 처음에 workflow request class를 실제 선택 scope까지 전달하지 않아 거부됐다.
   전송/역할/선택/결과 회수의 모든 관련 경로를 수정했고 native 시험 통과.
3. maxTurns 시험 장치에서 TaskOutput 사용 불가, 일시적인 종료 파일과 gateway 소비의 경합,
   비동기 완료 후 추가 부모 회차의 fixture 응답 누락을 발견했다. transient file 대신
   gateway의 보존된 실제 종료 관측을 사용하고 추가 부모 회차는 최대 3회로 제한했다.
   최초 timeout들은 maxTurns 미지원의 증거가 아니었다. 실제 TUI에서는 빈 결과 처리 결함을
   별도로 확인·수정했다.
4. 첫 TUI `338e7ba3-4324-4b30-9c1b-246539ad372a`는 부모의 불필요한 로그 조회에서 Read 실패/
   Glob 거부가 발생했으며, 조사 대기 중 launcher 제한 시간에도 도달했다. 합격이 아니다.
   native는 유예 안에 회수됐다. 수정된 TUI 시나리오와 구분한다.
5. 전체 회귀는 설명의 옛 `rejects tools and maxTurns` 문구를 기대한 테스트 1개가 실패했다.
   새 기능/기존 native schema 보존을 검사하도록 갱신했고 gateway 일반/race 각 553개 통과.
   나머지 16 packages는 원래 전체 실행에서 통과. 처음 실행을 전체 PASS로 바꾸지 않았다.
6. 첫 live non-streaming 시험은 요청 class 헤더가 없어 기존 컨텍스트 정책이 거부했다.
   필요한 class/session을 명시한 공개 API 시험으로 수정한 뒤 4개 모델 모두 통과했다.
7. `95b3180`의 clean build TUI `2895cf89-c811-4796-9c42-4baaa94dedde`는 실제 불합격이다.
   3개 자식의 도구 제한은 지켜졌지만 일반 inline 자식 B/C가 자신의 computed task 대신
   부모 전체 검증 지시를 답변에 재현했다. B는 Read를 실행하지 않았고, C의 B 성공 문구는
   증거가 아니었다. 부모는 이를 성공으로 인정하지 않았다.
   [첫 최종 빌드 실패 자료](final-first-failure.json)를 보존한다.
   확인한 구현 공백은 계획 step에만 넣었던 worker 역할 안내가 raw inline 자식에는 빠진 것이었다.
   기존 안내를 출처가 확인된 모든 Workflow 자식에 공통 적용하도록 수정했다.
   native/user 지침이나 computed task 본문은 바꾸지 않는다. 변경된 지침도 모델의 모든
   지시 준수를 기계적으로 보장하지 않으므로 아래 동일 복합 TUI로 재검증했다.

최종 관련 검사: Workflow 76개, gateway 553개, gateway race 553개,
새 settings/Workflow native race 14개, vet exit 0. skip 및 package별 결과는 evidence.json 참조.
한 번의 통과를 모든 환경의 비플래키 보장으로 확대하지 않는다.

### 최종 제품과 TUI 확인

`313a78cfe99a84fb43d7f1c25a9fe4a9bad07f09`에서 `vcs.modified=false`로 빌드했다.
[build.json](build.json)과 [final-evidence.json](final-evidence.json)이 소스·바이너리·TUI를 연결한다.
`clauduct.exe` SHA256은 `fdc613eb07bfe1362729ff4df947a406d93f4b0808a86c04661905c56e406a3d`다.

실제 최종 TUI `4ce18162-9c5f-4c1d-9647-df0c2da00ee2`에서 실패했던 같은 프롬프트를 보냈다.
정확히 3개 자식이 만들어졌으며 A는 Read 1회 후 빈 결과를 미확보로 보고,
B는 실제 Read 1회와 `S48_READ_OK`, C는 도구 없이 `43 S48_NO_TOOLS_OK`를 반환했다.
B/C는 부모 전체 검증 지시를 재현하지 않았다. 완료 이벤트 후 부모가 세 결과를 받아 답했고,
다음 같은 세션의 일반 입력에 `S48_NEXT_TURN_OK`로 응답했다. API 오류 0, native 도구 오류 0,
재실행 0, 정상 종료/회수. 의도된 빈 결과의 미확보 1건은 성공 본문으로 대체하지 않았다.

`95b3180` 전체 회귀는 1,639 통과, 17 packages, 3 skip이다. 마지막 worker 범위 보완 뒤에는
영향받는 Workflow 76개, gateway race 553개 및 vet가 통과했다. 전체 회귀를 마지막 변경 후
다시 실행했다고 표시하지 않는다. skip은 콘솔 강제 종료, live 전체 세션 opt-in, launcher
강제 종료 수동 검사이며 이번 범위의 별도 TUI 실행과 동일 검사가 아니다.

clean `95b3180`의 필터 TUI도 `d15ae515-a1c7-4d67-ad05-24127a739f65`에서 정상 취소·회복했다.
관측 간격은 1,272ms였다. 이후 `313a78c`는 worker 역할 안내만 제품 변경했으며 필터 경로를
변경하지 않았다. 두 필터 시험을 모든 환경의 지연 상한으로 일반화하지 않는다.

검증한 파일 3개를 `D:\AIDEV\clauduct-s36-build`의 개발 경로에 반영했다.
[promotion.json](promotion.json)에 해시와 백업 경로가 있다. 설치된 release, 전역 설정,
AdGuard 설정은 변경하지 않았다. 반영은 확인된 개발 기능의 사용을 위한 것이며 공개 출시가 아니다.

## 아직 남은 구현과 검증

- launcher 재시작 후 Workflow 복원: 메모리의 origin/run/child/중단 증거를 지속·재검증하는
  구현이 아직 없다. 저장된 script만 재생하는 방식은 이미 시작한 효과의 중복 금지와 충돌한다.
- named/scriptPath/nested Workflow: 파일 범위·hash·부모/run/child 연결과 native 경로별 검증이 필요하다.
- 커스텀 역할에서 모델 생략, 직접 Workflow maxTurns 옵션의 독립 구현, 모든 native 옵션 조합은 미지원/미검증이다.
- sampling/정확 생성 상한은 위 backend 거부 조건에서 실제 적용 경로가 확보되지 않았다.
- 모든 새 모델/멀티모달/종료 환경, native 표시의 내부 원인, 임의 필터의 영구 전면 차단은 전체 성공 보장 대상이 아니다.

따라서 이 보완의 범위별 확인과 전체 기능 완성은 별개다. 미구현 항목을 구현 불가능으로
바꾸거나, 현재 테스트의 성공을 v0.3.0 전체 최종 합격으로 올리지 않는다.
