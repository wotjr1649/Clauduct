# Session-29 로컬 출하 근거

시작: 2026-09-14T04:14:56Z. 마감: 2026-09-14T07:14:56Z. 초기 후보 `f425cf0ead479a7914ce9b4381bbcff854fae13f`, branch `work/unattended-release-2026-09-13`. 현재 판정 **HOLD**. 구현 worktree의 tracked 상태는 clean이었다. 사용자 루트 README +2/-0와 기존 untracked 자료는 보존한다. 되돌림은 이번 작업의 개별 커밋/이해한 diff 단위로만 가능하며 사용자 branch에 통합하지 않는다.

## 예산과 실행 경계

기준 원장 `.tmp/release-completion-20260914/live-agent-final/result.json`을 직접 대조했다. 요청 323~327, 보수적 327; 관측 입력 1124010/출력 47690; 미관측 입력 예약 786432/출력 예약 196608; 예약 포함 입력 1910442/출력 244298. 관측 시간 3019404ms와 미관측 160000ms도 유지한다. 추가 한도 요청64/입력400000/출력30000, 누적 상한391/2310442/274298이다. 담당 Codex 사용량과 구분한다.

기존 fixture의 입력131072/출력32768은 완료 후 검사다. `src/native-protocol.mjs`는 `usage-enforced-completion`을 선언하고 upstream에 출력 토큰 상한을 보내지 않는다. 일반 Responses API의 `max_output_tokens` 문서는 Codex 구독 endpoint의 지원 증거가 아니다. 기존 PoC에는 해당 인수를 backend가 거부하는 계약/실패 이력이 있다. 생성 중 reasoning을 포함하는 출력 상한을 사전 보장하지 못하므로 새 실호출은 보류한다. 완료 후 관측, 짧은 prompt, 연결 중단을 소비량 상한의 증거로 쓰지 않는다. 관련 독립 로컬 작업은 계속한다.

## 현재 요구 대조

과거 F 표의 IN_PROGRESS를 최신 범위에 그대로 복사하지 않는다. 아래 PASS는 명시된 검사 범위에 한하며 기능 전체나 다른 후보의 결과를 뜻하지 않는다. 실행 근거의 hash와 변경 영향은 최종 artifact에 추가한다.

| 요구 | 현재 근거 / 적용 범위 | 현재 남은 차단 |
|---|---|---|
| F01 연결·DNS·TLS·단절 | DNS 공개 주입과 오류 분류, 공개 native 출력 복구. TLS 실제 준비 실패 이력 유지 | HTTP/TCP 제외. 핵심 복구 증거 영향 대조 |
| F02 Retry-After | parser/defer/새 프로세스 복구 PASS. 자연 quota 대기 불필요 | 변경 영향 대조 |
| F03 반복503 | 실제 native 공개503 및 두 모델 효과 뒤 서비스 오류/동일 세션 복구 | 변경 영향 대조 |
| F04 HTTP200 오류 | 두 조합 공개 native 오류·미완료 도구 차단·동일 세션 복구 | 변경 영향 대조 |
| F05 전달 후 프로토콜 오류 | 두 조합 공개 native 단절/UTF-8/순서 오류와 원래 보고서 완료 | 변경 영향 대조 |
| F06 crash·효과 확인 | local-native 읽기/쓰기 뒤 중단·동일 세션 재개, 개발 부분 적용 검사 | 필요한 복구 경계별 근거 대조 |
| F07 pipe·bounded 수집 | 실제 모델 개발과 공개 native 출력 단절/재쓰기0 | 변경 영향 대조 |
| F08 인증 | 합성401/cache 교체/만료/예산 경계 PASS | 정상 갱신은 사용자 제외 |
| F09 철회·계정 경계 | 합성401반복/403/계정 변경/cache 오류 거부 | 실제 계정 조작 불필요, 보호 유지 |
| F10 기본 압축 | 설정·변환·라우팅 근거 유지 | 기본400K/320K 실발동은 사용자 제외 |
| F11 소유·중복 재개 | 배타적 소유/공개 프로세스 경쟁·재시작 정산 중 시작0 | Workflow 이전 worker 생존 경계 추가 필요 |
| F12 부모·자식 완료/실패 | 일반 Agent 및 고정 성공 main relay 두 모델 성공 | 일반 결과/실패·중복·오래된 결과·형제·취소 연결 검사 필요 |
| F13 취소·형제·늦은 응답 | 현재 소스의 공개 native4사례, CANCELLED/형제완료/잔여0 | 선택 변경 회귀 |
| F14 Workflow 재개 | 신규 run·긴 journal·출처 검사. 정상 재개 구현 없음 | 동일 저장 세션/새 프로세스/저장 결과 재사용·필요 agent 재실행/파일 효과 증거 필요 |
| F15 메모리·queue·기한 | admission 기한/회복 PASS. native 동시성 ECONNRESET/cleanup 간헐 실패 | HTTP 이력은 제외하되 admission 독립 회귀 필요 |
| F16 기록 실패·잘림 | v2 원장·미관측 예약·변조/부분 적용 거부 | 기존 registry 등록 거부 보존. disk-full 실제 증거 미확인 |
| F17 종료·고아 | 이전 실제 실행 cleanup9/잔여0 | 이번 새 실행마다 소유자/자식 종료 확인 |
| F18 압축·재시작·자식 실패 결합 | 기존 표 NOT_RUN, 개별 압축과 복구는 대체 증거가 아님 | 기본 압축 제외와 별개로 결합 증거 미확인 |
| F19 hooks/permission/MCP | native 허용/거부·효과0/1 및 로컬 계약 | 현재 약속 설정 범위의 회귀 대조; 임의 개인 구성 전면 보증 아님 |
| F20 악성 입력 | 소스/도구 경계·MCP 쓰기 전 거부·독립oracle | 외부 전송 없는 로컬 음성 검사로 유지 |
| F21 model/effort | 두 조합 일반 Agent/신규 Workflow/개발 실제 성공 | 새 재개 경로 model/effort 검증 필요 |
| F22 경로·위조·기록 재사용 | 기존 정적 경로·신원·변조 보호 | 새 재개 음성 사례 필요. 동적 링크는 사용자 제외 |
| F23 oracle/가짜성공 | 부분 적용/검사 삭제·변조/결과 불일치 거부 | 관련 코드 불변 여부 대조 |
| 기능·설정 | 기존 기능표/옵션표 및68파일 선택 회귀 | 전체 적용 필수 항목과 최신 후보의 영향 대조 |
| 릴리즈 | 기존 ZIP은 다른 commit | 커밋 후보 ZIP·manifest/hash·새 경로 실행 필요 |

## 진행

- 목표 도구에 기존 목표가 없어 최신 목표를 등록했다. 전체 PASS 전에는 완료로 표시하지 않는다.
- 초기 task 관련 프로세스 census에서 실행 중 native/node worker는 관측되지 않았다. 검사 명령 자신의 PowerShell만 확인했다.
- 추가 실제 요청/입력/출력: 0/0/0. 이전 미관측 예약은 그대로다.
- F12 제품: TaskOutput 구조화 결과를 원본 호출/현재 child request/metadata/마지막 응답에 결합하고, 메인의 동일 부모 SendMessage 재개에서 원자적으로 소비한다. 29개 새 검사 및 기존 선택/실패/다중알림 검사가 통과했다. 공개 native sol 실패·성공, luna 실패·취소4사례는 같은 부모/세션, 자식 시작2개, 보고서 쓰기1회, cleanup9/잔여0을 독립 대조했다. `.tmp/session-29-release/relay-native-verified.json`. 최초 wrapper의 상태 필드명 오류/검증기 project 디렉터리 가정 실패는 유지하고 원자료를 재검증했다.
- 넓힌 gateway 검사는 22개 뒤20초 watchdog에서 실패했다. 변경 전 f425cf0을 별도 새 디렉터리에 추출한 기준선도 정확히 같은22개 뒤 watchdog 실패다. `.tmp/session-29-release/gateway-baseline.json`과 `relay-related-tests.json`. 이를 새 코드의 회귀 PASS나 HTTP 수정 완료로 표시하지 않는다.
- F14 원자료 대조: 이전 공개 probe의 구조화 반환 scriptPath와 실제 재호출 인수는 문자 단위 동일(356자, backslash14개)했다. 정확한 반환 경로를 native 허용 읽기 범위 검사가 거부했다. 오류를 경로 파싱 착오라고 추정해 재시도하지 않는다. 새 프로세스 동일 저장 세션 정상 Workflow 재개는 아직 입증할 수 없어 차단 상태다.
- F12 변경 커밋: `2ac9963a3ea18fad09686031f7d6da4ccc3b4033`.
- 출력32768 고정값을 낮출 수 있게 headless/development 실행·예약·fixture 사용량·PowerShell 판독의 한도를 연결했다. 기본값과 과거 원장은 유지한다. `-MaxOutputTokens 30000 -RequirePreGenerationLimit`는 현재 전송의 한계를 명시적으로 거부한다. 실제 headless 진입 preflight에서 실행 디렉터리 생성0/native시작0/credential0/실요청0을 확인했고 `.tmp/session-29-release/live-preflight-blocked.json`에 남겼다. 이 옵션을 빼고 같은 실호출을 진행하지 않는다.
- 예산 검사19개, PowerShell 한도 판독3개 및 기존 관련10파일이 통과했다. 낮은 한도(각 단계 입력10000/출력3000)로 두 조합의 공개 native 출력 단절→이전 worker 종료→새 프로세스 동일 세션 개발 마감을 확인했다. 각8개 고정 응답, sol5805ms/luna5497ms, 묶음 쓰기1/파일 쓰기2, 최초 OUTPUT_PIPE_CLOSED 보존, 실제 backend0이다. `lower-budget-recovery-sol.json`/`lower-budget-recovery-luna.json` 및 first/finish 원자료가 근거다. 일반 저장 세션 재개 성공이며 Workflow 재개 성공으로 대체하지 않는다.
- 예산 보완 커밋: `762b5e41562c3b24bba51f32a79a34c32d0ebd06`. 후속 영향 검사10파일 중9개 통과, headless 기존 실행 디렉터리 거부 검사가 실패했다. 새 토큰 preflight가 기존 경로 검사보다 먼저 실행된 것이 원인이다. 기존 경로/충돌 검사를 먼저 유지하고 파일 생성 전 토큰 preflight를 수행하도록 순서를 바로잡았다. 해당24개 검사와 예산19개 재검증 통과. 실패한 최초 묶음은 `candidate-impact-tests.json`, 수정 검사는 `preflight-order-fixed.json`에 보존한다.
- 기존 증거 manifest12개 hash가 일치했다. 과거 실모델 개발 두 실행의 wrapper 결과 hash·실행 결과·events hash를 대조했고 각각 독립81개 성공을 확인했다. 이전 서비스 오류 native 증거의6종류×2조합도 확인했다. 관련 핵심 전송·프로토콜·모델·admission·소스/oracle11파일은 f425cf0과 동일하다. `.tmp/session-29-release/evidence-reuse.json`에 재사용 범위를 제한했다. 최초 대조의 evidenceHash 대상 파일 착오는 수집기 원문을 읽고 바로잡았으며 실제 실행을 반복하지 않았다.

### 예산 인터페이스

`verification/verify-native-headless.ps1`의 `-MaxInputTokens`/`-MaxOutputTokens`와 `verifyNativeDevelopment()`의 `maxObservedInputTokens`/`maxObservedOutputTokens`는 기존 한도 이하의 **완료 사용량 제한**이다. `-RequirePreGenerationLimit` 또는 `requirePreGenerationLimit: true`는 현재 `VERIFICATION_PREGENERATION_LIMIT_UNAVAILABLE`로 실행 전 거부된다. 관측 한도를 설정했다고 생성량이 그 값에서 멈춘다고 주장하지 않는다. 일반 Responses API의 [max_output_tokens 정의](https://developers.openai.com/api/reference/typescript/resources/responses/methods/create)는 공개 API 계약이며 구독 backend가 이를 지원한다는 증거는 아니다.
