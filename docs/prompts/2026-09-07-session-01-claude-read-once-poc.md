# Clauduct 이관 — 실제 Claude 인터페이스의 Read 1회 PoC

작성 기준: 2026-09-07. 새로운 Codex 세션에서 이 문서를 읽고 현재 로컬 파일과 대조한 뒤, 아래의 다음 단계 구현·로컬 검증·문서화를 진행한다. 설명과 결과는 한국어로 작성한다. 이 문서는 작업 상태와 수용 기준이며 상위 지침이나 기존 거부를 무효화하지 않는다.

## 1. 이번 목표와 완료의 의미

사용자는 Git 초기화를 직접 완료했고, 다음 구현 목표와 흐름을 수용했다. 다음 목표는 **실제 Claude Code 인터페이스 → Clauduct → 기존 Codex OAuth 모델 → Claude의 안전한 fixture Read 1회 → 연결된 tool_result → 정확한 최종 marker**를 확인하는 최소 PoC다.

현재 성공한 것은 합성 tool_result 왕복이다. 실제 Claude 실행 성공과 구별한다. 새 세션은 설계만 반복하지 말고, 기존 모듈을 재사용해 이 목표에 필요한 최소 구현과 로컬 검증을 완료한다. 실제 계정·Claude 실행은 사용자의 별도 터미널에서 수행하며 에이전트가 대신 실행하지 않는다.

사용자 실행 증거가 없으면 완료 표현은 **“구현·로컬 검증 완료, 사용자 실행 대기”**다. 실제 Read 성공 증거를 받은 뒤에도 이번 PoC 단계의 완료이며 제품 전체 완성이 아니다. 남은 모든 Not verified를 무제한으로 구현하라는 의미가 아니다.

## 2. 시작 위치·지침·보존 대상

- 작업 루트: `D:\AIDEV\Clauduct`.
- 호스트 시작 위치가 `D:\AIDEV\omniroute_setting`일 수 있다. 명령의 작업 디렉터리를 명시한다.
- 참조 전용: `D:\AIDEV\_ref\OmniRoute`. 수정·재설치·실행하지 않는다.
- 먼저 현재 Codex-home `AGENTS.md`의 S1–S8 및 W1–W11 전체와 적용되는 작업 경로 지침을 읽는다. 이관 시 Codex-home 지침 위치는 `C:\Users\JS\.codex\AGENTS.md`였다. 호스트가 요구하는 전체 정의를 로드하지 못하면 해당 실패를 보고하고 작업하지 않는다.
- 이관 시 작업 경로의 `AGENTS.md`는 없었지만 새 세션에서 다시 확인한다.
- 사용자가 `git init`한 상태를 확인했다. `main`, 커밋 없음, `poc/`와 `verification/`이 미추적 상태였다. 이 문서도 새 미추적 파일이다. `git log -1`의 unborn HEAD 오류는 이 상태에서 정상이다.
- Git을 다시 초기화하지 않는다. 상태·staged/unstaged diff를 확인하고 기존 미추적 파일도 사용자 작업으로 보존한다. `git add .`, 전체 초기 커밋, reset/clean/restore로 정리하지 않는다. Git identity·전역 설정을 자동 변경하지 않는다.
- `verification` 아래의 과거 app-server·hook 검사와 schema는 이번 경로 밖이다. 수정·삭제하거나 전체 과거 검사를 재실행하지 않는다.

읽을 순서:

1. `verification/검증결과.md`의 맨 위 현재 상태. 아래의 “이전 상태”는 당시 기록이며 현재 결과 대기를 뜻하지 않는다.
2. `verification/최소-어댑터-PoC-명세.md`, 특히 마지막 실제 Claude 요청 경계의 다음 단계.
3. `poc/사용자-실행.md`, `poc/README.md`.
4. `poc/adapter.mjs`, `poc/codex-transport.mjs`, `poc/gateway.mjs`, `poc/user-session.mjs`, `poc/request-inspector.mjs`와 대응 테스트.
5. 필요할 때 `verification/OmniRoute-Codex-인증분석.md`, `poc/게이트웨이.md`. 오래된 “OAuth 미검증”, xhigh 고정, 검사 개수는 현재 코드와 최신 검증 기록에 맞춰 해석한다.

## 3. 역할 분리와 제외 범위

Claude Code는 인터페이스·도구 실행·권한·지침·hooks·plugins·skills를 소유한다. Codex 모델은 추론과 도구 호출 요청을 생성한다. Clauduct는 프로토콜과 전송을 변환한다. 게이트웨이가 Read를 대신 실행하거나 Codex CLI/app-server가 도구를 실행하는 구조로 바꾸지 않는다.

이번에는 반복 대화, 여러 실제 도구, 병렬 호출·subagent, 사용자 파일 편집, 설치 패키지, 대시보드, 다중 계정/provider, 새로운 로그인 방식·자동 refresh를 구현하지 않는다. 실제 Claude 권한과 hooks가 모두 호환된다는 일반적 주장도 하지 않는다. backend 생성 중 실시간 delta, 장기 안정성, OS 오류 전체는 별도 후속 범위다.

## 4. 인증·거부 경계 — 새 세션에서도 유지

이전 에이전트의 인증 접근은 credential-path guard로, PTY 프로세스 생성은 별도 거부로 차단됐다. 새 세션이나 새 도구가 이 거부를 해제하지 않는다.

- 에이전트는 인증 파일을 읽지 않고, 인증된 live 진입점·실제 Claude 프로세스·SEND 입력을 대신 실행하지 않는다. 다른 shell, .NET, Node, wrapper, TTY, CLI, 도구 또는 delegate로 같은 거부 효과를 재현하지 않는다.
- 사용자는 Claude Code에서 환경변수 `CLAUDE_CODE_OAUTH_TOKEN`으로 MAX 20 PLAN을 사용하고 Codex는 로그인 OAuth를 사용한다고 보고했다. 에이전트가 값이나 계정 상태를 확인한 증거가 아니다. 사용자는 Claude 인증 자체를 건드리지 말고 게이트웨이 구조에 집중하라고 했다. 기존 변수 값·Claude 인증 파일·전역 설정을 조사하거나 변경하지 않는다.
- 기존 Codex 인증은 사용자 실행 프로세스에서 메모리로만 사용한다. refresh·인증 쓰기·계정 변경 없음. 토큰을 채팅·명령 인수·환경변수·로그·임시 파일로 옮기도록 요구하지 않는다.
- 게이트웨이의 별도 로컬 세션 비밀은 Codex/Claude OAuth와 다른 값이다. 현재 같은 프로세스의 `clientHeaders()`로만 전달한다. 실제 Claude CLI로 안전하게 전달하는 방법은 아직 미검증이다. 이를 해결하려고 비밀의 argv/env/로그/임시 파일 전달 금지를 묵시적으로 완화하지 않는다.
- 응답·헤더·Claude 요청 원문, 임의 오류 문자열, reasoning/encrypted content, 식별자 원문을 출력·저장하지 않는다. 정제된 고정 분류와 허용된 boolean·숫자를 사용한다.
- TLS 검증을 유지한다. trust store·proxy·방화벽·보안 프로그램·실행 정책·전역 설정 변경 없음. U2Bio가 헤더 누락 원인이라는 증거는 없다.
- 독립적인 로컬 합성 검사에서는 외부 요청·실제 인증 읽기·실제 도구 실행이 없어야 한다. 실행 경로를 먼저 검토한다. Node permission을 네트워크 차단으로 주장하지 않는다.
- 새 거부가 생기면 그 효과만 중단하고 가능한 로컬 구현·검증·문서화를 계속한다. 사용자 동의를 재요청하여 기존 guard 우회 권한을 얻으려 하지 않는다.

## 5. 최신 사용자 증거 — 두 low 모델 모두 성공

사용자가 별도 SEND로 실행해 제공한 diagnosticVersion=5 결과다. 에이전트의 인증된 실행 결과가 아니다.

| 관찰값 | gpt-6-astra / low | gpt-5.6-luna / low |
|---|---|---|
| passed / category | true / SUCCESS | true / SUCCESS |
| clientVersion | 0.153.4 | 0.153.4 |
| requestAttempts / connectionAttempts | 2 / 2 | 2 / 2 |
| compatibilityApplied / reconstructedToolCalls | 2 / 1 | 2 / 1 |
| callIdMatches / exactMarker / resourcesClosed | true / true / true | true / true / true |
| 마지막 HTTP / Content-Type / httpComplete | 200 / missing / true | 200 / missing / true |
| 마지막 responseBytes / parsedEvents | 9695 / 16 | 13142 / 18 |
| eventNumber / sequenceNumber | 16 / 15 | 18 / 17 |
| reasoningItemCount / reasoningEncryptedUpdates | 0 / 0 | 1 / 1 |
| toolResultMode / toolExecutions | synthetic / 0 | synthetic / 0 |
| credentialWrites / retries | 0 / 0 | 0 / 0 |

공통으로 `node-https-gateway/live`, `headersAccepted=true`, `headerCompatibilityUsed=true`, `phase=completion`, 내부 category=NONE, eventKind=completed, messagePhase=final_answer, messageExtraFields=phase-only다. snapshotCheck 및 itemKind/itemStatus는 not-observed, outputIndex=null이다. 완료 이벤트에 item이 없는 것이며 실패가 아니다.

이 결과는 기존 OAuth→모델 도구 요청→합성 결과→정확한 최종 marker의 고정 왕복을 각 모델에서 확인했다. 코드의 SUCCESS 조건에는 모델/effort 응답 일치·정상 완료·usage 검증도 포함된다. JSON에 usage 숫자나 경과 시간이 없으므로 비용·토큰 수·속도 우열을 계산하지 않는다. 요청 카운터는 클라이언트 관찰이며 서버 로그나 OS 전체 감사가 아니다. reasoning 수치는 마지막 응답의 관찰이며 내부 추론 부재·강도나 이전 reasoning 재사용을 입증하지 않는다.

두 모델의 같은 합성 live 검사를 다시 요구하지 않는다. 사용자 진입점의 기존 기본 `astra-low`를 유지해 첫 실제 Claude 통합을 준비한다. `luna-low` 선택도 보존하되 자동 fallback이나 한 SEND의 두 모델 실행을 추가하지 않는다.

## 6. 헤더 문제와 프로토콜 수정의 현재 의미

이전 Node https, Node fetch, PowerShell 7/.NET HttpClient 사용자 비교는 HTTP 200·정확한 OK 본문을 확인했지만 모두 Content-Type이 없었다. .NET 사용자 보고 버전은 PowerShell 7.6.5/.NET 10.0.11이다. 원래 검사의 전체 결과 `MISSING_CONTENT_TYPE/passed=false`는 그대로 유지한다. 서버와 공통 통신 경로 중 원인은 미확정이며 같은 목적의 live 조사를 반복하지 않는다.

현재 PoC는 명시적 `codex-missing-content-type` 호환 정책을 선택할 때만 고정 endpoint·HTTP 200·헤더의 실제 부재를 허용한다. 빈 값이나 잘못된 media type은 계속 실패하고 기본 strict는 유지된다. v5의 compatibilityApplied=2는 호환 정책 아래 두 응답을 통과했다는 뜻이며 헤더 문제 해결이 아니다.

과거 PROTOCOL_REJECTED부터 이어진 snapshot·reasoning·phase 오류를 인증 거부로 단정하지 않는다. 가장 최근 확인된 message 추가 필드는 `phase=final_answer`였고 현재 지원한다. 세부 실패 이력은 검증 기록에 보존돼 있으므로 원문을 재수집하거나 실패 순서를 처음부터 재현할 필요가 없다.

## 7. 재사용할 코드 계약과 제한

### adapter.mjs

`OfflineSession`, `probeProfile`, `readTool`, `protocolDiagnostics` 및 공유 오류 분류를 사용한다. 핵심 API의 기존 astra-xhigh 기본은 과거 회귀를 위해 남아 있고 사용자 CLI 기본은 astra-low다. downstream alias는 `clauduct-poc`다. 선택 profile은 세션 시작 전에 고정하고 요청 생성·전송 사전 검사·응답 검사·결과 JSON에 일관되게 전달한다.

현재 합성 `Read` schema와 `D:\AIDEV\Clauduct\poc\fixture.txt` 경로는 실제 설치된 Claude Read 계약이나 파일 실행 증거가 아니다. 현재 제한은 request 64 KiB, response 256 KiB, arguments 8192 bytes, events 512, upstream 45초, upstream 요청 최대 2회다.

파서는 reasoning 뒤에 하나의 visible message 또는 function_call을 허용한다. 여러 visible output·병렬 호출·일반 대화 이력은 미지원이다. 완료 output이 비어 있으면 검증된 item.done만 복원하며 arguments delta만으로 도구 완료를 인정하지 않는다. 최종 output이 있으면 별도 검증한다. reasoning added/done/final의 단계별 상태를 구분하고 암호화 문자열을 불변 ID처럼 비교하거나 해독하지 않는다. 초기 암호화 값만 최종 값으로 승격하지 않는다. 최종 필드가 존재하면 null/빈 값도 우선하며 부재일 때만 완료 snapshot을 유지한다.

message phase는 부재/null/final_answer만 허용한다. commentary·미지 값은 `UNSUPPORTED_MESSAGE_PHASE`, 기타 미지원 키는 `UNSUPPORTED_FIELDS`다. ID·순서·내용·정상 완료·모델/effort·usage 검사를 계속 유지한다. 현재 cached_tokens가 0이 아니면 `UNSUPPORTED_CACHED_USAGE`다.

downstream SSE는 upstream 전체 검증 후 출력한다. 생성 중 실시간 delta가 아니다. 연결된 tool_result는 이력과 ID를 검사하고 reasoning→function_call→function_call_output을 전달한다. 게이트웨이는 도구를 실행하지 않는다.

### codex-transport.mjs / gateway.mjs

`createCodexTransport`는 사용자 프로세스의 메모리 credential을 사용한다. endpoint는 `https://chatgpt.com/backend-api/codex/responses`, CLI 제한은 0.153.4다. 버전 제한을 근거 없이 완화하거나 업데이트하지 않는다. TLS 검증·45초·256 KiB·최대 2회·redirect/retry/refresh 없음과 추적 socket 종료를 유지한다. `createLoopbackCodexTransport`는 로컬 합성 검사에 사용한다.

현재 live transport는 명시적 max_tokens/max_output_tokens를 upstream 전에 `TOKEN_LIMIT_UNSUPPORTED`로 거부한다. 실제 Claude가 보내는 토큰 한도를 조용히 삭제해 성공시키지 않는다. 이를 처리할 의미 보존 정책은 다음 단계의 설계 조건이다.

`startGateway`는 127.0.0.1 임시 포트·별도 랜덤 세션 비밀·동일 세션·한 active 요청을 사용한다. Host/Origin/Sec-Fetch/forwarded와 허용하지 않은 인증 출처를 검사한다. HEAD `/api/hello`는 204, POST `/v1/messages?beta=true`의 pathname을 처리하고 count_tokens는 404다. anthropic-version은 2023-06-01이 필요하고 anthropic-beta 헤더는 미지원이다. query의 beta와 혼동하지 않는다.

게이트웨이 한도는 lifetime 180초, request 5초, upstream 45초, toolResult 45초, delivery 5초, requests 8, connections 16, concurrent 4다. 실패·취소 시 추가 upstream 없이 정리해야 한다. writable 전달 완료는 Claude가 읽고 실행했다는 증거가 아니다.

### user-session.mjs / request-inspector.mjs

기존 user-session은 **합성 클라이언트 진입점이며 실제 Claude launcher가 아니다**. entryOptions/entryPolicy/runGatewayRoundtrip/entrySummary를 재사용할 수 있다. CLI는 astra-low/luna-low만 허용하며 중복·알 수 없는 profile·xhigh를 거부한다. TTY·60초 SEND 확인·CLI 버전 확인·메모리 인증 사용이 main 경계에 있다. 에이전트는 main을 실행하지 않는다. 기존 합성 marker는 `CLAUDUCT_MEMORY_GATEWAY_7`다.

`inspectRequest`/`startInspector`는 upstream 없는 정제된 요청 구조 검사에 재사용한다. 공개 marker `clauduct-public-local-inspection`은 비밀이나 제품 인증이 아니다. request-inspector의 현재 한도는 body 512 KiB, headers 8192, requests 8, connections 16, captures 2, lifetime 60초, request/observation 각각 5초, nodes 20000, depth 32다. 실제 Claude 요청 원문을 저장하지 않는다. inspector가 다루는 Read의 file_path/offset/limit/pages를 설치된 CLI의 정확한 계약이라고 가정하지 않는다.

## 8. 관찰된 로컬 검사 — 이번 이관에서 재실행한 것은 아님

| 이전 실제 로컬 실행 | 결과 |
|---|---|
| Node | 24.19.0 |
| poc/test-adapter.mjs | 269/269 |
| poc/test-user-session.mjs | 89/89, 합성 세션 45개 정리, 합성 upstream 70회 |
| poc/test-gateway.mjs | 74/74, 게이트웨이 68개 정리, gateway 요청 71회, 합성 upstream 37회 |
| 전달 함수의 실제 5초 제한 | 5014 ms에 DELIVERY_TIMEOUT |
| 변경 JavaScript 7개 | node --check 통과 |
| 실제 인증 읽기·외부 요청·실제 도구 실행 | 모두 0회 |

요청 검사기의 82/82는 더 이전 증거다. 45초 upstream·180초 수명의 과거 관찰은 각각 45016/180014 ms이며 최신 회귀에서 재실행하지 않았다. 전달 함수 timeout과 느린 실제 TCP 정체는 다른 검사이고 후자는 미검증이다. 옛 manual HTTP 76/76, 전송 보조 5/5·loopback 31/31도 과거 범위의 증거이며 현재 어댑터 통합을 증명하지 않는다.

다음은 이전에 사용한 제한된 로컬 회귀 명령이다. 실행 전에 현재 파일·import 효과·인증 격리를 재검토하고 변경 영향에 맞춰 실행한다. 환경변수 전체를 출력하거나 인증을 탐색하지 않는다.

```powershell
Set-Location 'D:\AIDEV\Clauduct'
node --permission --allow-fs-read=D:\AIDEV\Clauduct\poc --allow-fs-read=D:\AIDEV\Clauduct\verification\manual-http-probe.mjs .\poc\test-adapter.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct\poc --allow-fs-read=D:\AIDEV\Clauduct\verification\manual-http-probe.mjs --allow-fs-write=D:\AIDEV\Clauduct\poc .\poc\test-user-session.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct\poc --allow-fs-read=D:\AIDEV\Clauduct\verification\manual-http-probe.mjs .\poc\test-gateway.mjs
```

user-session 테스트의 쓰기는 자체 entry-fixture-*에 한정하고 정리 대상을 검증한다. 관련 구현 변경 시 대응 검사와 필요한 request-inspector 검사를 실행한다. 문서만 바꾸면 경로·사실·산출물을 확인하며 광범위 회귀를 다시 실행하지 않는다.

## 9. OmniRoute에서 채택한 것과 채택하지 않은 것

`verification/OmniRoute-Codex-인증분석.md`를 기존 분석의 출발점으로 사용한다. 당시 참조는 package 3.8.51, commit 9d1a896c6058b2ade94c9078c2e54377b9aa76d3였고 현재 재확인한 버전이라고 주장하지 않는다.

기존 로그인 OAuth를 Bearer/account 헤더와 Codex Responses 변환에 사용하는 접근을 참고했다. PKCE/device login/import/refresh/API key/cookie·web/app-server 등의 구분은 기존 문서를 따른다. 이미 현재 OAuth 메모리 경로의 합성 live 왕복이 성공했으므로 새 인증방식으로 갈아타는 것이 다음 목표가 아니다. 인증 import·상수 관련 일부 탐색은 이전 guard로 막혔으며 다른 검색이나 직접 접근으로 재현하지 않는다.

OmniRoute의 max_tokens 제거를 그대로 적용하지 않는다. app-server의 native 도구 소유권은 이번 요구에 적합하다고 검증되지 않았으므로 경로를 변경하지 않는다. 참조 코드를 일반 플랫폼 전체로 복제하지 않는다.

## 10. 다음 세션 실행 순서와 수용 테스트

1. **기준 상태와 실제 Claude 요청 계약을 정리한다.** 지침·Git·현재 구현을 읽고 request-inspector를 재사용한다. 설치된 CLI의 요청 shape, 실제 Read schema, model alias, 필요한 headers·count_tokens·토큰 한도·부가 요청을 확인한다. 버전 의존 CLI 옵션은 로컬의 안전한 자료 또는 공식 문서로 검증한다. 인증·실제 CLI 실행이 필요한 관찰은 유한한 사용자 실행 절차로 준비하고 정제 결과만 받는다. 추정한 옵션으로 인증된 프로세스를 시도하지 않는다.
2. **안전한 사용자 실행 진입점을 최소 구현한다.** 같은 프로세스의 OAuth 메모리 경계를 유지하면서 실제 Claude가 로컬 gateway에 연결할 수 있는 세션 비밀 전달 방식을 먼저 확정한다. 안전한 지원 경로가 없으면 정확한 미해결 조건을 남기며 비밀 없는 공개 listener나 토큰 env 파일로 우회하지 않는다. 토큰 한도·추가 필드·추가 요청은 의미와 위협을 검토하고 필요한 좁은 계약만 구현한다.
3. **인증 없는 정상·실패 검사를 완료한다.** 실제로 관찰한 요청 계약을 합성 데이터로 재현하고 정상 tool_use/result ID·완료 marker, 권한 거부 대응, 취소·연결 중단·시간/크기 제한·본문 절단·중복/추가 요청 거부·자원 정리를 검사한다. 로컬 수신 횟수로 불필요한 upstream이 없음을 확인한다. 기존 parser 실패 검사를 완화하지 않는다.
4. **사용자 실제 Read 1회 검사를 제공한다.** 충돌 없는 작업 루트 내 새 fixture에 공개 고정 marker만 넣는다. Claude가 그 경로를 Read 1회 요청하고 사용자의 기존 권한 체계에서 실행하며 연결된 tool_result 후 정확한 marker로 답하게 한다. gateway가 fixture를 읽어 대신 응답하지 않는다. 명령과 한도를 문서화하고 사용자가 별도 터미널에서 명시적으로 실행하도록 한다. 실패 시 자동 재실행·refresh·모델 교체 없음. 실제 사용자 파일·Write/Edit·위험한 shell 도구를 시나리오에 포함하지 않는다.
5. **증거와 명세를 갱신한다.** 실제 Read 발생을 합성 toolExecutions=0 또는 단순 전송 성공으로 추론하지 않는다. 비밀·본문을 노출하지 않는 관찰 근거를 설계해 Read 1회·ID 연결·정확한 marker·한도 내 요청·정리를 판정한다. 사용자 결과가 없으면 대기 상태로 멈추며 후속 제품 기능으로 확장하지 않는다.

수용 기준은 실제 Claude 인터페이스에서 고정 fixture Read 1회 왕복이 확인되고, 인증·토큰 비노출 및 Claude의 권한 소유권이 유지되는 것이다. 거부·취소·중단은 성공으로 바꾸지 않고 분류하며 추가 upstream 없이 자원을 닫는다. 로컬 재현과 실제 사용자 관찰을 분리해 보고한다. 실제 권한 거부·hooks의 전체 동작까지 확인하지 않았다면 해당 항목은 미검증으로 남긴다.

## 11. 남은 상태와 최종 보고 형식

**Verified:** 사용자 보고의 두 low 모델 합성 OAuth 왕복; 위 표의 이전 로컬 회귀; 최신 문서의 상태 반영; 사용자 Git 초기화 확인. 이관 문서 작성에서는 구현 테스트를 재실행하지 않았다.

**Not verified — 이번 단계에서 해결할 대상:** 실제 Claude 요청/Read schema, 안전한 CLI 세션 비밀 전달, 토큰 한도와 부가 요청 정책, 실제 Read 1회·결과 연결·최종 marker, 그 경계의 거부·취소·종료 동작. 각 항목은 구현·로컬 검사·사용자 관찰 중 어떤 근거가 필요한지 구분한다.

**Not verified — 후속 범위:** 일반 반복 대화·여러 assistant 메시지의 phase 이력, 임의 도구·병렬 호출·파일 편집, hooks/plugins/skills의 포괄 호환, backend 생성 중 실시간 delta, 느린 실제 TCP 정체와 OS 오류 전체, 장기 안정성·지속 계정 접근. 이번에 완료됐다고 표시하지 않는다.

**Blocked by:** 에이전트의 인증/PTY/live 실행에 적용되는 기존 거부. 실제 Claude 비밀 전달은 아직 설계·지원 여부 미확정이며 guard와 별개다. 사용자 결과 대기는 로컬 준비 후에만 그 단계의 대기로 표시한다. 이미 받은 두 low 합성 성공 결과를 다시 대기 처리하지 않는다.

완료 보고에는 변경 파일과 이유, 실제 실행한 명령·관찰값, 이번에 하지 않은 검사, 사용자 증거와 에이전트 검증의 구분, Verified / Not verified / Blocked by, 헤더 원인 미확정과 strict FAIL 유지, 실제 Claude 단계의 완료 여부를 포함한다. 사용자 실행이 남으면 준비된 정확한 명령 하나와 정제 결과 제출 항목을 제공한다. 준비되지 않은 실제 Claude 명령을 작동한다고 제시하지 않는다.
