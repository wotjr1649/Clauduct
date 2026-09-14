# Clauduct — Windows 릴리즈 안내

현재 후보의 무인 개발 출하 판정은 **HOLD**다. 로컬 ZIP 생성은 파일 구성·무결성 검사이며 전체 출하 검증의 통과를 뜻하지 않는다. 2026-09-13 후보에서는 정상 HTTP 일부가 재성공했지만 request-inspector, user-session watchdog, TCP 반닫기 검사에 실패가 남아 있다. 취소 검사의 metadata 준비 경쟁은150ms 지연으로 재현하고, 실제 취소 도착까지 준비 완료를 보류하는 fixture로 고쳐 관련5개 파일 회귀를 통과했다. 같은 loopback 반닫기 이상은 Clauduct·Node 없는 .NET 비교에서도 관측했으며 원인과 정상 환경에서의 재검증은 미완료다.

2026-09-14 변경된 출하 범위에서 정상 인증 갱신 판단은 사용자 Codex CLI 로그인 기준으로 분리하고, 기본 400K/320K 압축 발동은 실사용 검증으로 이관했으며, 4h/24h×3/72h 장기 시험은 제외했다. 아래 과거 검증 설명의 이 세 항목은 현재 출하 보류 사유가 아니다. `CLI_VERSION_UNVERIFIED`도 기존 기준 버전과 다르다는 비차단 안내이며, 그 안내 자체를 실패로 세지 않는다. 그 밖의 필수 기능 실패와 검증 차단은 유지한다.

실패한 자식의 부모 재개 검증은 gateway가 해당 자식의 현재 요청에서 기록한 실패와 native의 `isApiErrorMessage`/`stop_sequence` 기록을 대조하도록 보완했다. 세션·직접 부모 관계·metadata·시간을 재확인하고, 성공·실패 혼합 알림의 증거를 한 번만 소비한다. 단순 실패 문구, 오래된 요청, 위조된 완료, 중단된 자식, 다른 계정·세션의 기록으로 재개하지 않는다. 새23개 검사와 기존 완료·다중 알림·모델 선택·진단 관련5파일 회귀가 통과했다. 실제 native의 공개 응답 시험에서는 sol/low와 luna/max 모두 자식 실패가 메인에 전달됐지만 부모에게 직접 알림이 전달되지 않아 부모 자동 재개는 미완료다. 해당 시험에 실제 backend 요청·credential 읽기는 없다.

추가 공개 native 검사에서는 sol/low·luna/max 각각 두 모델 자식 중 하나를 번갈아 취소한4사례가 통과했다. 실제 Agent 반환 ID·TaskStop·TaskOutput·gateway 실패 기록을 대조했고, 형제 완료·늦은 이벤트 차단·정리9항목·잔여0을 확인했다. 실제 backend 요청은0이다. 사용자 일반 PowerShell과 직접 .NET Socket에서도 TCP 반닫기 응답 유실은 재현되어 출하 HOLD를 유지한다. 최신 범위와 실행 증거는 [출하 목표 기록](docs/release-completion-2026-09-14.md)에 있다.

TCP/HTTP에는 알려진 안정성 제약이 있다. 일반 사용의 성공 사례도 있지만, 특정 연결 종료 조건에서는 응답 유실이나 리셋으로 턴·자식 작업이 중단될 수 있고 일부 연결 정리는 약1초 지연될 수 있다. 발생 빈도와 원인 구성요소는 미확정이다. [실사용 영향과 근거](docs/tcp-shell-assessment-2026-09-14.md#실제-clauduct-사용에-미치는-영향)를 기록했으며 추가 OS 진단은 보류한다.

완료된 이전 Workflow의 script 편집이 별개 신규 Workflow 자식을 IDENTITY로 차단하던 검증 순서를 수정했다. 실제 소속 run의 script·metadata·journal 검증과 중복 신원 거부를 유지하며, sol/low·luna/max의 공개 native 연속 Workflow 실행이 통과했다. 캐시 없는 재개는 별도 미완료다.

## 시작

1. ZIP을 새 폴더에 풀고 `Clauduct` 폴더를 연다. 기존 설치·사용자 프로필에 덮어쓰지 않는다.
2. 그 폴더의 PowerShell에서 `./clauduct.cmd --dry-run`으로 인증 없는 구성 검사를 한다.
3. 작업할 프로젝트로 이동한 뒤 설치 폴더의 `clauduct.cmd`를 실행한다.

```powershell
& 'D:/Tools/Clauduct/clauduct.cmd' --model sol --effort xhigh
& 'D:/Tools/Clauduct/clauduct.cmd' -p --model luna --effort low --output-format json --max-turns 3 -- 'Reply with exactly READY'
& 'D:/Tools/Clauduct/clauduct.cmd' --resume <session-id>
```

`D:/Tools/Clauduct`는 예시 설치 위치다. PATH, 전역 설정, 서비스, 인증 파일을 설치기가 생성하거나 수정하지 않는다. 새 버전으로 바꿀 때도 별도 폴더에 풀면 이전 실행기로 되돌릴 수 있다. native 세션·프로필은 별도로 유지되며 두 버전을 같은 세션에 동시에 연결하지 않는다.

## 필요 환경

- Windows, PATH의 Node.js. 검증 환경은 Node `24.19.0`, native Claude `2.1.269`, Codex standalone `0.154.0`이다. 다른 조합의 성공을 보장하지 않는다.
- native Claude는 OS 사용자 홈의 `.local/bin/claude.exe`, Codex는 `AppData/Local/Programs/OpenAI/Codex/bin/codex.exe`를 사용한다. 임의 실행 파일 경로를 설정으로 받지 않는다.
- 기존 Codex 로그인과 OS 사용자 홈의 `.codex` 파일 credential store가 필요하다. Clauduct는 인증을 직접 갱신하거나 쓰지 않는다. account 변경·다른 `CODEX_HOME`·디버그/TLS 우회 런타임은 거부한다.
- 인증 값을 읽기 전에 루트 `cli_auth_credentials_store`를 구조적으로 구분한다. [TOML 1.0 키와 문자열 규칙](https://toml.io/en/v1.0.0#keys)에 따라 따옴표·Unicode escape·주석·여러 줄 문자열·중첩 값·테이블을 구분하며, 명시한 다른 저장소를 놓치고 파일 캐시를 읽던 경우를 수정했다. 이 읽기는 다른 설정을 적용하거나 전체 TOML 설정의 유효성을 보증하지 않는다. 입력65536자·중첩64단계·해석한 키/저장소 문자열1024자 한도이며, 모호하거나 지원하지 않는 인증 설정은 캐시 조회 전에 거부한다.
- PowerShell `7+`는 검증 스크립트에 필요하다. 별도 npm 의존성 설치는 없다.

Claude Code의 UI·로컬 도구·기존 권한 검사는 유지하고 모델 요청만 `127.0.0.1` gateway를 거쳐 ChatGPT Codex backend로 보낸다. Codex app-server 경로가 아니다. provider/secret 계열 환경 변수는 native 자식에 전달하지 않는다. 이 환경 변수에 의존하는 사용자 도구·MCP는 별도 호환성 확인이 필요하다.

## 동작과 오류 계약

- 기본 메인은 `astra/low`다. 명시 모델의 기본 effort는 astra/medium, sol/xhigh, terra/high, luna/max이며 `--effort`가 우선한다.
- `-p`/`--print`는 비대화형이다. stdout은 native 결과만, wrapper 안내·종료 상태는 stderr다. 명시적 print 없이 파이프로 실행하면 TTY 오류로 종료한다.
- 텍스트는 스트리밍하고 도구 호출은 정상 완료 검증 뒤 전달한다. 전달 전 일시 I/O·429·5xx는 제한 재시도하며 전달 후 오류는 자동 재실행하지 않는다. 완료된 도구를 반복하지 말고 `--resume`으로 이력을 이어간다.
- 검색도 HTTP 429 또는500~599에서만 상태 코드에 따른 재시도를 한다. 비표준600 이상을 일시 장애로 재시도하던 범위를 수정했으며, 로컬 HTTP의14개 경계 사례에서 model 최대6회·search 최대2회와 종료 후 자원 회수를 대조했다.
- DNS 오류는 `UPSTREAM_DNS_ERROR`로 분류하고 정해진 재시도 한도를 적용한다. 인증서·TLS 오류와 접근 거부는 `UPSTREAM_TLS_ERROR`·`UPSTREAM_ACCESS_DENIED`로 분류하여 즉시 종료한다. 모델과 검색에 같은 분류를 적용하며 인증서 검증은 유지한다. 요청의 `attempts[].failureCategory`는 재시도 후 성공한 요청에도 앞선 실패를 남긴다.
- 모델·검색의401 재시도는 취소 여부와 남은 요청 한도를 확인한 뒤 credential을 다시 조회한다. 비동기 조회 중 취소된 검색도 새 전송 전에 중단한다. 공개 합성 cache 파일의 교체·만료·계정 변경과 반복401/403을 로컬 HTTP에서 검사했으며, 실제 사용자 credential이나 정상 OAuth 갱신의 증거와 구분한다.
- 서버의 `Retry-After`가 짧은 대기 예산을 넘으면 `UPSTREAM_RETRY_DEFERRED`와 `retry_at_ms`를 반환한다. 그 시각보다 일찍 재개하지 않는다. 숫자 범위로 표현할 수 없는 유효한 지시는 `UPSTREAM_RETRY_UNREPRESENTABLE`로 표시하고 자동 재시도를 막는다.
- 메모리 admission은 진행 중 요청을 유지하며 새 요청을 기본 최대 30초 대기시킨다. 기한을 넘으면 upstream 실행 전에 HTTP 503과 `MEMORY_ADMISSION_TIMEOUT`을 반환한다. 상태의 `admission`에 큐 길이·대기 상한·기한 초과 누계가 남으며, 여유 메모리 회복 뒤 다시 요청할 수 있다.
- JSON `is_error`, 종료 코드, `CLAUDUCT_REQUEST_STATUS`의 `requestOutcome`·`failureHistory`·`cleanup`을 함께 확인한다. exit 0이나 `Clauduct 종료: SUCCESS`만으로 세션 중 모든 요청이 성공했다고 판단하지 않는다.
- 종료 상태는 설치 폴더의 `.clauduct-status/request-status.jsonl`에도 추가된다. 원문 오류·인증 값은 기록하지 않는다. 기록 실패는 명시적으로 안내하며, 강제 프로세스 종료에서는 기록을 보장하지 않는다.
- 비대화형 자동화에는 `--max-turns`와 호출자의 시간 제한을 둔다. `--bg`/`--background`의 세션 분리 동작은 이 릴리즈에서 실검증되지 않았으며, 검증된 background 도구/TaskOutput/TaskStop과 구분한다.

[CLI 옵션 경계](docs/claude-option-classification.md)에 차단·전달 범위를 정리했다. 차단 옵션이나 Anthropic 전용 서비스는 지원 기능으로 간주하지 않는다.

## 확인된 기능과 남는 한계

이전 후보의 실측: 비대화형 JSON·stream-json, Read/Edit, Bash·PowerShell, stdio MCP, PNG 입력, WebFetch·WebSearch, Agent 선택, inline Workflow·StructuredOutput·부모 복귀, background 도구 결과 회수·TaskStop 뒤 worker 종료, 새 프로세스 resume, 실패 후 도구 미중복·이력 보존·정리를 확인했다. 모델 네 종류의 low 요청도 당시 성공했다. 이전 후보의 결과를 이번 후보 전체의 회귀 통과로 복사하지 않는다.

2026-09-13 작업 중 후보별 실측: `luna/max`와 `sol/low`에서 실제 코드 구현 및 독립28-case 판정, 고정 효과 직후 native tree 중단과 동일 세션 자동 재개, Agent·Workflow 자식 라우팅, PNG/JPEG/GIF/WebP, WebFetch 내부 요약과 WebSearch를 확인했다. JPEG/GIF/WebP는 native Read 결과와 실제 upstream MIME도 각각 일치했다. 모든 기능이 최종 동일 후보에서 재검증된 상태는 아니며, 최초 실패 및 관측기 수정 전 실패를 별도 원장에 보존한다.

짧은 검증기의 guarded fixture는 실제 전송 시도 직전과 완료 사용량을 새 작업 경로의 제한된 숫자 원장에 기록한다. 두 조합에서 정상 종료 출력과 원장이 일치했고, 첫 응답 직후 강제 종료에서도 마지막 숫자를 복구했다. sol의 종료 주입은 잔여 프로세스0까지 통과했다. luna의 즉시 종료 관측은 실패로 보존했으며 이후 잔여0을 확인했다. 진행 중이던 요청의 미관측 사용량은0으로 처리하지 않는다. 전원 손실 내구성이나 장기 streaming 수집을 입증한 것은 아니다.

검증 원장 v2는 완료 응답에 사용량이 없거나 잘못된 경우 `unobservedCompletions`를 별도로 기록한다. 이때 `INVALID_USAGE`로 도구 응답 전달과 이후 모델/검색 요청을 중단한다. 명시적인0 사용량은 정상 관측과 구분하며, 이미 관측된 한도 초과량도 기록한 뒤 전달을 거부한다. 이전 v1 원장은 읽을 수 있으나 v2 증거로 바꾸어 부르지 않는다. 개발 결과에는 고정 실패 분류·사용량이 관측되지 않은 시도 수가 포함된다.

`verify-native-headless.ps1`의 `text`와 `stream-json`도 도구 없는 검증 정책에서 시도·사용량 원장을 기록한다. 판독기는 v2의10개 요약 필드와 종료 출력의8개 필드를 대조하며, 중복·알 수 없는 필드·잘못된 숫자와 불일치를 거부한다. 종료 출력이 없으면 남은 원장의 관측값과 미완료 상태를 보존한다. 검증 PASS에는 v2 마감·사용량 관측·종료 확인이 필요하다. 선택적 `-RunId`는 소문자16진수32자리로 새 실행 경로를 미리 정하며, 이미 존재하는 경로는 실행 전에 거부한다. 이 기록 보강과 단문 성공은 실제 개발·장기 단계의 증거를 대신하지 않는다.

개발 검증기는 응답 헤더의 기본 Codex 사용량 두 구간도 숫자로 기록한다. [공식 Codex의 헤더 해석](https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/rate_limits.rs)을 기준으로 사용률·분 단위 기간·초 단위 재설정 시각을 구분한다. 시각은 부호 있는 안전한 정수로 보존하며 음수를 미래 시각으로 해석하지 않는다. 잘못되거나 중복된 필드는 null과 고정 오류 분류로 남기고, 독립적으로 유효한 다른 숫자는 보존한다. 추가 구간도 같은 여섯 숫자만 최대8개 관측하고 전체 개수와 초과 여부를 별도로 남긴다. 식별자를 보존하지 않으므로 응답 간 배열 위치를 같은 구간의 식별자로 사용할 수 없다. 원문 헤더·구간 이름·계정·plan·credit balance는 기록하지 않는다. 기록 callback의 실패나 비동기 반환은 `RESPONSE_OBSERVER_FAILED`로 해당 응답 전달과 후속 모델·검색·credential 재조회를 중단한다.

2026-09-13 기본 구간만 관측하던 이전 runtime의 `sol/low`와 `luna/max` 실제 deadline 과제는 각각5요청으로 독립32-case 검사·v2 원장·응답 관측·회수를 통과했다. 추가 구간 관측을 구현한 현재 실행에서 sol/low는2요청/10524ms 뒤 task 읽기만 마치고 종료해 개발 FAIL, luna/max는5요청/87417ms로 소스 검토·독립32-case 검사·v2 원장·회수 PASS다. 두 실행 모두 추가1구간의 여섯 숫자를 관측했으나 기본 secondary 재설정 시각의 형식 오류는 남아 있다. 계정 전체 quota나 잔여토큰을 확정하지 않는다. 로컬 검사는 추가 구간34개·기존 숫자/증거102개·실제 loopback17요청/57개이며, 앞선 실패와 관측 공백도 보존했다. Node test 환경에서 실제 HTTPS factory의 옵션을 검사하려던 음성 시험은 기존 runtime guard에 차단됐으며, 원래 test body와 명시적skip을 보존했다. 일반 native 개발 성공으로 이 차단 시험까지 통과했다고 세지 않는다. [공식 사용량 안내](https://learn.chatgpt.com/docs/pricing)의 메시지 추정치를 현재 계정 잔여량으로 대체하거나, 이 관측으로 요청 예산을 자동 증액하지 않는다.

단일 과제의 `--local-task <model> <pwsh.exe> <task-id> [requestLimit]` 및 `--live-task <model> <pwsh.exe> <task-id> <prior-attempts> <prior-elapsed-ms> <prior-input-tokens> <prior-output-tokens> [requestLimit]`는 선택적 마지막 인수로1~16 요청 상한을 지정한다. 생략 시 기존16이며 `verifyNativeDevelopment`의 같은 옵션은 예약과 실제 transport 상한을 함께 제한한다. native entry는 두 값의 일치를 확인한다.6요청 상한의 로컬 native 두 조합은 각각5요청으로 완료했고, 위 실호출도 이 상한을 적용했다. 시간·토큰·누적 상한을 자동으로 늘리는 옵션은 아니다.

`-Case text -FailFirstConnection` 검증에서는 첫 연결 전에 DNS 오류를 한 번 주입한다. `luna/max`와 `sol/low` 각각에서2시도 중 실제 모델 응답1개로 원래 요청을 완료했고, 첫 실패 분류·이후 성공·원장·정리를 확인했다. 연결 오류 주입의 로컬51개 검사는 모델과 검색의 TLS/접근 거부 시도1회, DNS 재시도 한도와 고정 진단을 검사했다. 실제 자체 서명 TLS 서버를 사용하는 별도 시험은 준비 단계에서 종료되어 아직 미검증이다.

`verify-native-service-recovery.mjs --local-native`는 실제 native와 loopback HTTP를 사용하고 모델 응답은 고정 공개 fixture로 공급한다. 두 조합 설정에서 반복503, HTTP200 error, 텍스트 전달 후 단절·잘못된 UTF-8·순서 오류의10개 사례를 확인했다. 실패한 응답의 미완료 도구는 실행되지 않았고, 오류 종료 뒤 관리기가 같은 세션을 재개하여 영수증 조회와 원래 보고서를 완료했다. 효과 호출·조회·보고서 호출은 각각1회이며 첫 실패와 최종 성공을 따로 보존한다. 이 검사는 실제 backend 장애나 장기간 서비스 대기·인증 복구의 증거를 대신하지 않는다. fixture의 선택적 `audit` 모드는 도구 이름만 효과 전에 기록하고, 기록 실패 시 효과를 실행하지 않는다.

공용 검증 관리기의 `WAITING`은 검증된429/5xx 분류와 재개 시각을 실패 exitCode와 같은 동기 원장 행에 보존한다. 새 프로세스에서도 기한 전에는 worker를 시작하지 않으며 이후 기록으로 서버의 더 늦은 기한을 줄이지 않는다. `deferred-503` 사례에서는 두 조합 설정의 실제 native가503+Retry-After60을 받고, 새 관리기의 이른 재개는worker0으로 대기한 뒤 같은 세션에서 원래 보고서를 완료했다. 이 두 시험은 실제 시간을 각각60초 이상 기다린 로컬 서비스 시험이며 모델 전송·credential 조회는0이다. 기록 완료 후 결과 출력 전 프로세스 종료, 잘못된 상태·타입·분류와 미확정 효과의 재실행 거부도 검사했다. 응답을 받았지만 아직 대기를 기록하지 못한 crash 구간과 전원 손실은 미검증이다.

`verify-native-recovery.mjs --live <model> <pwsh.exe> <prior-attempts> <prior-elapsed-ms> --service-signal`은 기존 로그인으로 실제 모델을 호출하는 별도 검사다. `luna/max`와 `sol/low`에서 효과 영수증 뒤 다음 전송 직전에 합성503 분류를 주입하고, 같은 세션을 자동 재개해 상태 조회와 원래 보고서를 완료했다. 각 조합은 실제 backend4요청, 효과·조회·보고서 각각1회였으며 오류 종료와 최종 성공, 지정 라우팅, 숫자 원장, 프로세스 회수를 대조했다. 주입된 신호는 `backendAttempted=false`로 기록하고 실제 backend HTTP503 응답이나 요청 사용량으로 세지 않는다. 도구 가드는 각 단계의 고정 도구와 빈 인수만 허용한다. 이 짧은 검사는 장기 서비스 중단이나 임의 개발 도구의 복구를 대신하지 않는다.

`native-output.mjs` 수집기는 JSON/stream-json을 행마다 검사하고 중간 메시지를 누적하지 않는다. 두pipe의 정상 EOF, 유일한result/종료상태, native exit와 독립oracle이 일치해야 완료다. 실제 로컬 자식6개의 느린 소비·32MiB 이상 중간 출력·큰result·과대result·pipe 단절·잘림을 검사했으며, 효과와보고서가 있어도 결과가 누락되면완료=false였다. 소규모 복구 검증기에는 총1MiB·한행1MiB·4096레코드 상한으로 연결했다. 두 조합 설정의 실제 native stream-json에서도 전달 후 장애·오류result·동일 세션 재개·최종result와cleanup을 확인했다. 장기 관리기 전체는 아직 미검증이다.

`verify-native-task-isolation.mjs --local-native <model> <alpha|beta> <pwsh.exe>`는 두 고정 background worker 중 지정한 하나만 취소하는 검사다. `luna/max`와 `sol/low` 설정에서 취소 대상을 바꾼 네 사례가 통과했다. 실제 native의 Bash 결과와 call ID를 대조해 TaskStop 대상을 고정하고, 대상 PID 종료 후에도 다른 worker가 살아 있으며 TaskOutput으로 정상 완료되는지 확인했다. 두 worker의 제한된 파일 권한, 정확한 도구 호출, result/상태 각1개와 회수를 함께 검사했다. native가 background ID를 worker의 첫 기록보다 먼저 반환하므로, 아직 없거나 작성 중인 시작 기록만 최대1500ms 기다린다. 이 시험의 모델 응답은 로컬 공개 fixture이며 실제 backend·credential 사용은0이다. 모델을 실행하는 Agent 자식의 취소와 늦은 upstream 응답 격리까지 검증한 것은 아니다.

추가 회귀의 `test-cancel-snapshot.mjs`에서15초 timeout이 한 번 관측됐다. 단계 표시를 넣은 별도 관측본은 기존6개 검사를 통과했지만 원래 실패의 원인은 미확정이다. 원본 검사에도 고정 단계명과 완료 검사 수를 timeout 진단에 추가했으며, 시간 한도와 기존 assertions는 유지했다.

실제 개발 검증기는 `native-development-entry.mjs`의 도구 가드·전송 전 숫자 원장과 bounded stream-json 수집기를 사용한다. 고정 과제별로 제한한 문법만 소스 쓰기에 허용하며, 그 뒤에도 외부 개발자의 해당 해시 소스 검토와 독립 oracle이 필요하다. 소스 형식 검사는 일반 JavaScript sandbox가 아니다. 소스 검토 대기·도구 순서·한 응답 한 도구·고유 call ID를 유지했고, 공개 parser의 독립28-case 검사에서 두 조합의 실제 기준 코드 실패→모델의 소스 작성→검토→검사 성공을 확인했다. 각5요청으로 luna는66776ms, sol은84442ms에 통과했고, 결과·상태 각1개/두pipe EOF/숫자 원장/독립oracle/회수가 일치했다. 모델 응답이 고정된 별도 `--local-native` 시험도 두 조합에서 통과했다. 이 단기 재검증은 장기 단계의 개발 횟수나 임의 코드 작업의 전체 보안 증거로 세지 않는다.

`development-tasks.mjs`는 기존 `retry-after-seconds`와 새 `retry-delay-window`의 소스 파일·함수·문법·요구사항·판정기를 고정한다. 과제 ID는 실행 전 budget과 재개 검증에 결속하고 모델 인수에서 변경할 수 없다. 새 과제는 잘못된 timestamp, 지난 마감, 마감 시각과 같거나 더 늦은 재시도를 거부하고 서버의 대기 시간을 임의로 줄이지 않는 순수 함수를 구현한다. `verify-native-development.mjs --live-task <model> <pwsh.exe> retry-delay-window <prior-attempts> <prior-elapsed-ms> <prior-input-tokens> <prior-output-tokens>`로 검증한다. 실제 luna/max는145761ms, sol/low는86966ms에 각각5요청으로 기준 실패→모델 구현→검토→독립32-case 성공을 완료했다. 시간에는 외부 소스 검토 대기가 포함된다. 두 실행 모두 결과/상태·EOF·원장·소스 해시·회수가 일치했다. 잘못된 과제 선택·다른 과제 소스 쓰기·미검토 실행·다른 과제의 재개 거부를 포함한67개 검사도 통과했다. 새 과제의 로컬 native 정상 실행과 같은 세션의 출력 단절 복구는 통과했으며, 실제 모델의 새 과제 출력 단절과 여러 개발 과제를 이어가는 장기 관리기는 아직 미검증이다.

`verify-native-development.mjs --live-sequence <model> <pwsh.exe> <prior-attempts> <prior-elapsed-ms> <prior-input-tokens> <prior-output-tokens>`는 완료된 parser 과제에서 새 deadline 과제로 같은 native 세션을 한 번 이어간다. 이전 소스·oracle·사용량 원장·소유자 종료를 대조하고 누적 사용량 초기화와 중복 successor를 거부한다. 이번 시험의 공개 config 경로만 재사용하며 이전 함수와 assistant 완료 응답의 존재를 전송 전에 확인한다. 이전 WIP의 실제 luna/max는10요청/211613ms로 통과했다. sol/low는 첫 과제 세 번째 요청의 `keepalive` 이벤트에서 실패해 두 번째 과제를 시작하지 않았다. 원래 실패 이벤트의 본문 구조는 미관측이다. 현재 지원하는 빈 keepalive 형태와 나머지 거부 조건은 아래와 같으며, 원래 실패가 해소됐다고 판정하지 않는다. 이후 동일한 최신 runtime 소스로 sol/low는200985ms/input36150/output988, luna/max는208838ms/input42088/output2306에 각각10요청으로 두 과제를 완료했다. 소스·동일session·독립oracle·v2 원장·회수가 일치했으며 실제 heartbeat는 네 phase 모두0개였다. 원래 이벤트의 실측 재발 검증과 장기 일반화·과제 인계 도중 crash 복구는 미완료다.

`verify-native-development.mjs --live-output-recovery <model> <pwsh.exe> <prior-attempts> <prior-elapsed-ms> <prior-input-tokens> <prior-output-tokens>`는 모델의 소스 작성과 독립 검사 성공 뒤 최종 출력 파이프를 끊고 같은 시험 세션을 자동 재개한다. 두 실제 조합에서 각8요청으로 luna110248ms, sol97471ms에 통과했다. 첫 실행은exit0이어도result 누락으로실패다. 첫 실행의 종료상태 메시지는 관측하지 못했으며, 숫자 원장과 별도 프로세스 관측으로 종료를 대조했다. 재개 전후의source·TASK·oracle·MCP 설정·구현 해시와 이전 소유자 종료를 확인하고, 새 소유자 기동 전에 한 번만 생성할 수 있는 재개 의도를 기록한다. 재개 도구는원래 요구사항/소스 읽기와 검사 각1회로 제한하여 소스 쓰기총1회, 동일session/source, 최종result/상태각1개·두EOF·cleanup·독립28-case·원장 일치를 확인했다. 공개 고정 응답을 사용하는 `--local-output-recovery`도 두 조합에서 통과했다. 이 증거는 해당 소스 효과를 조회할 수 있는 짧은 개발 과제에 한정하며, 장기 시험이나 임의 도구의 복구를 대신하지 않는다.

연속 개발과 출력 단절 재개의 최상위 `usageUnobservedAttempts`는 각 phase의 원장과 시도 집계가 일치할 때만 합산한다. 누락·잘못된 숫자·원장 불일치·불완전한 시도 집계·합계 초과는 `null`이며, 재개 호출이 결과 없이 실패하면 `continuationUsageUnobserved=true`를 함께 남긴다. 이 경우 기존 예약을 관측된0으로 해제할 수 없다. 집계 경계51개와 관련 인수23개·출력 장벽8개가 통과했다. 해당 집계 수정의 실제 native+loopback 출력 단절 재개는 sol/low5805ms·luna/max5923ms로 각각8요청, 연속 개발은 sol/low3897ms·10요청으로 통과했고 최상위/phase 집계·원장·회수를 대조했다. 이 변경의 실제 backend 요청은0이며 과거 실제 모델 증거를 새 소스의 실호출로 합치지 않는다.

`verify-native-development.mjs --local-incomplete-recovery <model> <pwsh.exe>`는 요구사항 읽기 뒤 정상 종료한 과제를 `DEVELOPMENT_TASK_INCOMPLETE`로 남기고, 같은 새 공개 fixture·native 세션에서 원래 과제를 한 번 재개한다. 재개 전에 루트·모델·과제·세션·구현/기준 소스·TASK/oracle/MCP·원장·누적 사용량·이전 소유자 종료를 대조한다. 읽기 기록1개만 있고 테스트/소스 쓰기가 없는 경우에 한정한다. 재개 의도를 새 소유자 기동 전에 한 번 기록하며, 소스 검토와 독립 판정은 그대로 적용한다. 예약 검증을 연결한 로컬 native 검사는 sol/low5483ms·luna/max5039ms,각7loopback으로 첫 실행FAIL·재개PASS·같은session·소스쓰기총1회·원장/회수를 확인했다. 이 시험은 외부 모델 요청0이며, 실제 sol 조기 종료의 원래 기록을 복구 성공으로 바꾸거나 다른 후보의 세션을 이관하지 않는다. 실제 backend 복구와 장기 관리기 연결은 미완료다.

개발 검증기는 native의 각 실행 단계 전에 요청·입력/출력 토큰·정리 시간을 포함한 시간 상한을 `execution-reservation.json`에 한 번 기록한다. native entry도 실제 budget의 해시와 예약량을 확인한 뒤 시작한다. 정산은 별도 파일에 기록하며, 정산이 없거나 잘렸으면 예약 전량을 유지하고 미관측 숫자는 해당 항목의 예약을 유지한다. 관측된 초과량도 보존한다. 재개 때 이전 예약·정산·관측 파일을 다시 대조하므로 관측 파일을 변경한 공개 fixture는 후속 실행 의도 생성 전에 거부됐다.

`d999c8b`의 예약64개·실제 공개 자식3개의 crash18개·entry 거부24개와 기존 개발 관련 검사를 합한396개 확인 항목이 통과했다. 당시 출력 단절 복구는7134ms/8loopback, 연속 과제는5206ms/10loopback으로 통과했다. 이 증거는 프로세스 crash와 각 실행 단계의 보존에 한정한다. 전원 손실 내구성과 현재 수정본의 실제 backend 실행은 미검증이다. 호출자는 기존 전역 원장과 과거 미관측 예약을 유지해야 하며, 이 파일만으로 새 실행이나 한도 증액이 승인되지 않는다.

`execution-account.mjs`는 검증된 과거 잔액과 미관측 예약을 초기 잔액으로 보존하고, 계정 안의 다음 실행 디렉터리를 배타적으로 생성해 한 소유자만 예약하도록 한다. 정산 파일이 생겼어도 종료 증거를 확인해 기록하기 전에는 예약 전량을 유지하며, 이미 관측된 초과량도 낮추지 않는다. 종료 기록은 정산 상태와 native 결과의 hash에 결속한다. 살아 있는 소유자, 미완성 실행, 깨진 정산·종료 기록은 다음 예약을 차단한다. 파일 기록의 crash 구간이 불명확하면 실행을 추가하지 않으며, 전원 손실 내구성은 미검증이다.

`managed-development.mjs`는 이 계정을 native 검증기에 연결한다. 합성 검사에서는 `createExecutionAccount(root, { basisHash, previous, limits })`를 사용할 수 있다. 기존 원장을 연결하는 경로는 아래 `createManagedLedgerAccount`이며, root는 프로젝트 `.tmp` 바로 아래의 새 `managed-development-XXXXXX` 디렉터리다. CLI `--local-task <root> <model> <pwsh.exe> <task-id>`는 한 과제를 실행하고, `--local-continue`는 같은 계정의 직전 완료 과제와 같은 세션에서 다른 과제를 실행한다. `--reconcile <root>`는 기록된 native 소유자의 종료·현재 소스·사용량 원장·예약 결속을 확인해 정산한다. 정산을 위해 native를 다시 시작하지 않는다. 각 실행은6요청 상한이며, 계정 생성 자체가 모델 요청이나 한도 증액을 승인하지 않는다.

로컬 중단 주입 `--local-interrupt-after-result`는 native 결과 기록 후 관리기를 exit73으로 종료한다. 두 조합 설정에서 예약 전량 보존→새 관리기의 정산→후속 개발을 각각10loopback으로 확인했다. sol/low의 두 native 단계는6685ms, luna/max는5914ms였다. 초기 합성 잔액37시도는 유지됐고 두 과제의 관측10시도만 추가됐다. 각 과제의 소스 쓰기1회·독립 검사·같은session·사용량·회수를 대조했다. 실제 공개 프로세스7개의 소유 경쟁/중단42개 및 계정39개를 포함한 관련482개 확인 항목이 통과했다. 별도 공개 fixture는 다른 실행의 binding과 정산 후 변경된 결과를 거부했다. 기존 출력 단절 복구6506ms/8loopback과 조기 종료 재개5181ms/7loopback도 통과했다.

`--local-hold-after-source`는 공개 소스를 쓴 직후5초 동안 응답을 보류하는 로컬 중단 주입이다. 이 구간에서 관리기만 종료된 실행은 `--local-recover <root> <model> <pwsh.exe> <task-id>`로 원래 과제의 마무리를 검증할 수 있다. 새 관리기는 이전 관리기 종료와 예약 결속을 확인하고, 기존 소유권 검사기로 native 트리 상태를 확인한다. 회수 의도를 먼저 한 번 기록하며, 검사기 실패·응답 유실 후 다른 종료 수단이나 반복 회수로 진행하지 않는다. 원래 `result.json`은 없는 상태로 보존하고, 별도 중단 증거는 `taskCompleted=false`와 미관측 사용량의 전량 예약을 유지한다. 효과 소스·원래 실패 검사·TASK/oracle·원장·현재 구현 hash를 대조하고 독립 검사를 통과한 경우만 같은 세션의 마무리를 시작한다. 마무리 단계의 소스는 hash로 봉인하며 MCP 쓰기 재전달도 거부한다.

93312bf의 두 조합 설정의 실제 native+loopback에서 관리기 중단→원래 과제 마무리→후속 두 과제를 확인했다. sol/low의 마무리와 후속 단계는3175/3237/3575ms, luna/max는2450/2759/3272ms였다. 각 마무리3요청·후속 과제당5요청, 같은 세션과 원래 새 공개 config, 과제당 소스쓰기1회·독립 검사·소스30개와 예약 결속을 대조했다. 초기 합성37시도에 첫 실행의 전량 예약6과 확인된 후속13시도를 반영해 최종56을 유지한다. 이 실행들에서는 관리기 종료 시 native가 살아 있었으나 소유권 검사 시점에는 이미 종료되어 회수 대상0이었다. 관리기만 사라진 뒤 native가 계속 살아 있는 모든 경계까지 통과한 것은 아니다.

처음 세 번째 과제는 공개 fixture가 예전 `call_id`를 재사용하여 실패했다. 실행 인스턴스별 ID를 부여한 뒤 반복 과제·재개 이력은 정상 통과하고, 중복 ID의 기존 거부도 유지됐다. 별도 정상 복구 fixture의 중단 증거 hash를 변경하면 `INTERRUPTION_RECORD_CHANGED`로 거부되며 잔액은 줄어들지 않는다. 관련17개 파일/789개 확인 항목은 실패0이었다. 기존 출력 단절 복구는7595ms/8loopback, 읽기 후 조기 종료 복구는5641ms/7loopback으로 통과했다. 이 결과는 외부 모델·credential 추가 호출0이며 원래 실제 실패를 대체하지 않는다.

`--local-hold-after-read`는 요구사항 읽기 직후 관리기 중단을 시험한다. `--local-recover`는 소스 쓰기 전/후를 구분한다. 쓰기 전에는 읽기와 실패한 기준 검사만 허용하며, 원래 기준 소스가 그대로이고 독립 검사에서도 예상대로 실패해야 재개할 수 있다. 중단 증거 v2는 `stage=before-write`, `independentPassed=false`, `taskCompleted=false`와 전량 예약을 보존한다. 같은 작업 디렉터리·세션에서 기준 검사→수정→독립 검사를 수행하고 `result-retry.json`에 기록한다. 재개 후 소스 변경은 원래 중단 증거 hash에 결속된 실제 완료 결과가 있을 때만 인정한다. 완료 없이 바뀐 소스와 다른 중단 증거를 가리키는 재개 결과는 거부한다. 기존 기록과 다른 구현 버전의 실행은 현행 증거로 승격하지 않는다.

요구사항 읽기 직후 실제 native 실행 중 관리기만 종료한 공개 검사는 두 조합에서 재개와 후속 두 과제를 통과했다. sol/low는7621/2843/2800ms, luna/max는7594/2547/2091ms였다. 원래 결과 부재·원래 기준 검사 실패·같은 작업 디렉터리와 세션·수정1회·전량 예약을 대조했다. 재개와 후속 과제는각5loopback이며 초기 합성37+첫 예약6+관측15=58을 유지한다. 관련18files/843개 확인 항목, 실제 공개 fixture의 소스/증거 변조 거부, 기존 쓰기 후 복구와 출력 단절·조기 종료 복구도 통과했다. 외부 모델/credential 추가 호출0이다. 소스 쓰기 도중의 부분 효과나 모든 crash 경계의 일반화를 입증한 것은 아니다.

`managed-ledger-account.mjs`의 `createManagedLedgerAccount({ sourceDirectory, ledgerHash, manifestHash, scopeRoot, localNative })`는 검토한 기존 `result.json`과 `manifest.json`의 hash를 확인하고 허용된 숫자·boolean18개만 초기 계정에 반영한다. 실제 모드는 `localNative: false`와 명시적인 작업 루트 `scopeRoot`가 필요하다. 관측하지 못한 토큰·시간 예약은 초기 잔액에 포함해 보존한다. 파일당64KiB 제한, 중복 최상위 키, 알 수 없는 계수, 불일치·초과·변조를 검사한다. 원장 디렉터리와 작업 범위 registry의 배타 등록으로 같은 snapshot을 별도 계정에 재등록하지 못하게 한다. `openManagedLedgerSource(sourceDirectory)`는 기존 등록을 조회하며 초기화하지 않는다. 부분 등록과 원장 변경은 실패로 남기며 자동 재생성하지 않는다.

관리기의 `--live-task`, `--live-continue`, `--live-recover`는 이 등록을 요구한다. 공개 localNative 계정의 실제 모드 사용과 등록 없는 실제 실행은 예약 전에 거부한다. 등록된 원장은 정산 때도 다시 검증하며 기존 한도나 초기 예약을 낮추지 않는다. 이 인터페이스의 구현과 실제 모델을 통한 실행 성공은 별도 증거다. 이전 단발 native 검증기의 호출자가 관리하던 전역 원장까지 이 인터페이스가 자동으로 이관하는 것은 아니다.

2026-09-14 공개 검사는 계정43개·등록 경쟁/결과 없는 등록 완료24개·복사한 snapshot 중복 거부3개를 포함한다. 관련21files/911개 확인 항목 통과 뒤 live-mode 공개 메타데이터 검사를 추가한 계정43개도 통과했다. 두 조합의 공유 계정에서 세 과제는각5loopback으로 완료됐고, luna의 후속 과제는 같은 세션을 유지했다. 별도 sol 실행은 관리기exit73 후 예약 전량을 유지하고 새 native 시작0으로 정산했다. 공개 manifest의 한도 변경은 정산을 거부하며 예약 전량을 보존했다. 각 실행의 기존 PASS와 별개로 Node `--permission` 아래 사후 종합 종료 재확인은 `DEVELOPMENT_ACCOUNTING_INVALID`로 실패했다. 파일 증거4단계·각31개 소스 hash 대조는 통과했으나 종료 재확인 실패를 대체하지 않는다.

이 계정 경로의 추가 실제 backend 요청은0이다. 기존 실제 잔액297/한도301을 적용한 앞선 검사에서6요청 예약은 실행 디렉터리 생성 전에 거부됐다. 실제 원장 등록은 작업 범위 registry의 디렉터리 생성에서 Node `FileSystemWrite` 검사에 거부되어 운영 계정이 생성되지 않았다. 원장·한도·미관측 예약은 불변이며 이 등록을 다른 권한이나 경로로 재시도하지 않았다. 실제 누적 원장의 운영 경로 이관, 결과 없는 crash의 나머지 경계, 임의 개발 과제와 장기 시험을 수행하는 관리기 전체는 미완료다.

`createManagedPlan(root, { model, localNative, taskIds, maxDurationMs })`는 아직 새 실행이 없는 계정에 검토한 과제 목록을 저장한다. 기존 누적 초기 잔액은 유지한다. 목록·모델·모드·계정 hash·구현 hash·생성 시각·절대 마감 시각을 `development-plan.json`과 완료 표식에 연결하며, 부분 생성과 변경된 계획을 자동 재생성하지 않는다. 과제는 최대256개이고 마감은 최장72시간이다. 재시작할 때 마감을 연장하지 않으며 다음 과제의 전량 시간·사용량 예약이 들어갈 때만 시작한다. 빈 슬롯·undefined·null을 과제 목록에서 거부한다. 처음 undefined 항목은 기본 과제 처리와 JSON 변환이 달라 잘못된 계획을 기록했으나, 기록 전 문자열 검증으로 수정하고 실패 fixture를 보존했다.

`node verification/managed-plan-entry.mjs --local-plan <root> <pwsh.exe>`는 저장한 목록을 차례로 실행한다. `--live-plan`은 실제 모드로 등록한 계정과 계획이 필요하다. 각 실행의 예약에 계획 hash와 과제 순서를 넣으며, 계획이 있는 계정은 순서 정보 없는 단발 실행을 거부한다. 완료 여부는 계정 종료 기록과 실제 native 증거에서 다시 계산한다. 완료 뒤 관리기의 출력이 사라져도 해당 과제를 다시 실행하지 않는다. native 결과만 남은 경우 정산하고, 원래 결과가 없는 중단은 기존 소유권·중단 증거를 검증한 뒤 원래 과제를 복구한다. 그 밖의 실패는 다음 과제로 건너뛰지 않는다. 로컬 `--local-interrupt-after-step <root> <pwsh.exe> <count>`는 지정한 완료 단계 뒤 관리기를 exit73으로 종료하는 검사다.

공개 검사에서 sol/low의 세 과제 자동 실행, luna/max의 첫 과제 완료 후 관리기 종료→남은 두 과제 실행, 완료 계획 재조회 시 native 시작0, 소스 쓰기 전 관리기 종료→원래 과제 복구→다음 과제를 확인했다. 기존 예약·같은 세션·과제당 쓰기1회·마감 시각을 대조했다. 관련24files/1068개 확인 항목에는 공개 자식4개의 생성 경쟁/결과 유실, 변경된 계획, 만료된 합성 계획, 부분 기록과 누적 한도 거부가 포함된다. 이 검사는 외부 모델·credential 추가 호출0이며 전원 손실 내구성을 입증하지 않는다.

기본 과제 `retry-after-seconds`와 `retry-delay-window` 외에 검토한 순수 함수 과제를 등록할 수 있다. `verification/registered-development-tasks.mjs`의 `registerDevelopmentTask(text, expectedHash)`는 canonical JSON에 최종 줄바꿈을 더한 바이트와 SHA256을 대조하고 `registered-<sha256>` ID를 반환한다. 호출자는 등록 전에 전송할 요구사항·기준 소스·검사 데이터를 검토해야 한다. 해시는 바이트의 동일성을 고정하며 권한이나 검토 승인을 대신하지 않는다. 기록은 프로젝트 `.tmp/development-task-registry`에 한 번 생성하고, 기존 기록의 변경·잘림·잘못된 구조는 덮어써 수리하지 않고 거부한다.

등록 형식의 필드는 `version: 1`, `functionName`, `requirements`, `baseline`, `fields`, `numbers`, `strings`, `cases`, `localFixtureSource`다. 전체64KiB·요구사항8KiB·소스8KiB·검사1~128개의 한도가 있다. 각 검사는 고유한 `name`, `inputKind`, `input`, JSON scalar `expected`를 가진다. 입력은 JSON 또는 명시적인 `undefined`/`nan`/`positive-infinity`/`negative-infinity`다. 판정기는 이 데이터를 JSON 문자열로 삽입한 고정 프로그램으로 생성하며 native의 쓰기 범위 밖에 둔다. 등록 데이터로 명령·경로·실행 권한·전송 대상을 선택할 수 없다. 함수의 파일명은 `implementation.mjs`이며 기존 어휘 검사·외부 소스 검토·독립 검사를 모두 유지한다. 실제 과제 정의에는 `localFixtureSource: null`을 쓸 수 있고, 로컬 응답 fixture 시험에는 검토한 참조 소스가 필요하다. 참조 소스는 실제 모델의 과제 프롬프트에 포함하지 않는다.

공개 예약 잔액 과제의 독립37-case 검사와 등록 경로 검사를 추가했다. 관련26files/1170개 확인 항목이 통과했으며 MCP 자식4개에서 기존 Node `--allow-child-process` SecurityWarning4건을 별도로 집계했다. 코드처럼 보이는 JSON 문자열은 실행되지 않았고, 검토 전 실행·입력/전역 변경·임의 명령·예약어 함수명·등록 뒤 기대값 변경을 거부했다. 첫 구현은 함수명 `arguments`를 받아들였으나 실제 module 구문 검사에서 실패했다. 이 실패를 보존하고 기록 전에 거부하도록 수정했다. 재개·완료 대조는 등록 요구사항과 판정기 바이트까지 재검증한다.

실제 모드 계획은 같은 과제의 반복을 거부하며 등록 과제의 설명·검사 이름·검사 순서만 바꾸어도 같은 작업으로 판정한다. 이 비교가 임의 프로그램의 의미적 동일성이나 유의미한 개발 완료를 증명하지는 않는다. 로컬 반복은 세션·복구 검사에만 사용하며 계획 완료도 `longStageEvidence=false`·출하 HOLD로 보고한다. 등록된 순수 함수 과제는 일반 다중 파일 프로젝트 변경의 공급·검토·통합을 완성한 것이 아니다. 압축·정상 갱신·병렬·취소 등 필수 사건의 원자료 연결, 실제 backend 및 장기 단계 실행은 미완료다.

완료 결과에는 소스·요구사항·판정기·검토 기록·MCP 설정·도구 기록의 여섯 항목 hash를 저장한다. 다중 파일의 소스 hash는 전체 파일을, 도구 기록 hash는 이벤트와 쓰기 intent·복구 claim·재개 claim·완료 기록의 실제 바이트를 함께 포함한다. 관리기의 정산·계획 완료 조회와 다음 과제의 세션 연결은 현재 파일 바이트가 그 증거와 일치하는지 확인한다. 검토되지 않은 소스나 변경·잘림·경로 불일치는 `DEVELOPMENT_ARTIFACT_CHANGED`로 거부한다. 파일 hash 자체는 코드 실행·검토 승인·테스트 성공을 뜻하지 않는다. 앞 후보69dd14c에서는 native 성공 후 소스를 실패하던 기준 코드로 바꾸어도 완료 계획으로 조회되는 결함을 새 공개 fixture에서 재현했다. 수정 후 소스·검토 기록·도구 기록 변경의 native 검사에서 완료 판정을 거부하고 잔액과 추가 실행0을 확인했다. 관련27files/1194개 확인 항목이 통과했으며, 최초 실패와 변경된 fixture를 복원하지 않고 보존한다. 이것은 작업 산출물의 완료 증거 보강이며 실제 프로젝트에 적용한 변경의 검증이나 장기 단계 완료를 대신하지 않는다.

`verification/development-change.mjs`는 완료 결과를 적용 대상에 연결한다. `prepareDevelopmentChange({ accountRoot, entryIndex, targetRoot, targetPath, expectedBeforeHash, expectedAfterHash })`는 정산된 성공 실행의 원장·산출물을 다시 확인하고, 적용 전 파일이 과제의 기준 소스와 일치하는지 검사한다. 대상은 검증기와 분리한 프로젝트 `.tmp/development-target-<6자리>` 아래의 명시적인 `.mjs` 파일이다. 바깥 개발 담당자가 입력과 적용 대상을 검토한 뒤 호출하며, hash는 그 바이트의 동일성을 나타낸다. 함수는 새 `.tmp/development-change-<6자리>`에 `before.mjs`, `proposed.mjs`, `change.json`, `ready.json`을 만들고 대상 파일은 변경하지 않는다. 기존 사용자 작업을 포함한 일반 경로의 덮어쓰기나 임의 명령 실행을 제공하지 않는다.

바깥 담당자가 제안을 검토·적용한 뒤 `verifyDevelopmentChange(changeRoot)`를 호출하면 적용된 파일의 hash를 대조하고 기존 고정 판정기를 그 파일에서 실행한다. 환경을 비우고 정확한 소스·판정기 읽기만 허용한 Node 자식의5초·16KiB 출력 한도, 종료 코드·실패 수·검사 전후 파일 동일성을 검사한다. 검증 결과가 이미 있으면 새 검사를 시작하지 않으며, 검사 intent만 남았거나 일부 결과가 유실되면 `DEVELOPMENT_CHANGE_VERIFICATION_UNCERTAIN`으로 거부한다. `readDevelopmentChange`는 대상·제안·원래 완료 증거·검사 영수증의 현재 상태를 대조한다. 미적용·변경된 대상, 경로 이탈, 위조한 정상 종료와 검사 수를 완료로 처리하지 않는다.

`prepareDevelopmentChangeSet({ changeRoots, integration })`는 같은 계정·같은 대상 프로젝트의 검토된 제안2~16개를 묶는다. 각 제안은 서로 다른 원본 파일과 대상 파일을 가리켜야 하며 처음에는 모두 기준 상태여야 한다. 다중 파일 과제에서는 같은 실행 항목의 서로 다른 `sourcePath`를 묶을 수 있다. 이 함수도 실제 대상 파일을 변경하지 않는다. `readDevelopmentChangeSet`는 미적용·부분 적용·전체 적용 후 미검증을 구분한다. 일부 파일만 적용된 경우 `verifyDevelopmentChangeSet`는 검사 시작 전에 거부한다. 전체 적용 후에는 각 파일의 기존 판정기를 대조하고 실제 대상 파일들을 함께 사용하는 통합 판정기를 실행한다. 새 검사에는 각각5초·16KiB 출력 한도를 적용하고, 전체 결과가 이미 있으면 추가 검사0이다. 검사 intent만 남은 경우는 미확정으로 보존한다.

`integration`은 `{ version: 1, steps, cases }` 형식의 검토한 데이터다. `steps`의 각 항목은 `{ targetPath, argument }`, `cases`의 각 항목은 `{ name, inputs, expected }`다. 인수는 `{ kind: 'input', index }`, 앞 단계 결과인 `{ kind: 'result', index }`, 원시 상수인 `{ kind: 'literal', value }`, `{ kind: 'object', fields }`, `{ kind: 'sum', left, right }`로 구성한다. 안전한 정수끼리의 합만 계산하며 다른 합은null이다. 경로는 묶음에 들어간 대상에서만 선택하고 모든 대상을 적어도 한 번 사용해야 한다. 향후 결과 참조·임의 실행식은 허용하지 않는다. 단계32개·사례128개·입력16개·표현식 깊이8 및 전체 정의32KiB 한도다. 판정기는 마지막 단계 결과와 기대값, 입력 불변을 대조한다. 모듈의 기존 순수 함수 소스 경계와 검토를 유지하며 임의 프로젝트 코드 실행 권한으로 확장하지 않는다.

묶음은 제안·대상·정의·고정 판정기·개별 검사·통합 검사 기록의 현재 hash를 함께 확인한다. 검증된 파일 하나의 결과만으로 여러 파일 변경을 완료 처리하지 않는다. 공개 Retry-After 파서와 deadline 함수의 두 파일 적용에서 부분 적용 거부·개별28/32개·통합21개 검사를 확인했다. 이 증거는 파일 묶음의 적용과 조합 검사 범위이며, 일반 프로젝트 전체나 실제 모델의 다중 파일 개발·장기 단계 완료를 뜻하지 않는다.

`retry-project` 과제는 한 native 실행에서 `retry-after-seconds.mjs`와 `retry-delay-window.mjs`를 함께 읽고 수정한다. `write_source`는 `{ files: [{ path, code }, { path, code }] }`를 받으며, 두 경로와 순서를 고정하고 전체 소스8KiB 및 각 함수의 기존 언어 검사를 첫 쓰기 전에 적용한다. 검토 hash는 실제 두 파일 내용을 합쳐 계산하며 판정기 hash에는 별도 보관한 두 의존 판정기의 내용도 포함한다. 개별28/32개와 두 함수를 함께 쓰는21개를 합한81개 검사를 통과해야 완료다. 파일 집합·순서·경로·소스·검토·판정기 변경과 일부 쓰기 기록을 완료 증거로 받아들이지 않는다.

이 과제의 제안은 `prepareDevelopmentChange`에 명시적인 `sourcePath`를 추가해 파일별로 만든다. 제안의 v2 기록은 원래 실행 전체의 산출물과 선택한 파일·개별 과제·판정기를 함께 연결한다. 기존 단일 파일 제안은 v1을 유지한다. 두 모델 설정의 공개 응답을 사용하는 실제 로컬 native에서 한 과제의 두 파일 수정·81개 검사와 출력 중단 뒤 같은 세션의 완료를 확인했다. 이 복구에서는 두 파일을 각각 한 번 썼다. 일반 프로젝트의 임의 다중 파일 변경 및 실제 backend 개발 증거는 미완료다.

다중 파일 쓰기는 첫 효과 전에 최대32KiB의 `source-write-1.json` intent를 만든다. 제안 전체·과제·작업 경로·소유자·이전 소스 hash와 각 파일의 식별자·수정 시각을 결속한다. 기존 두 번의 수정 한도 안에서 두 번째 intent까지 허용하며, 완료된 동일 제안의 재호출은 쓰기0이다. 파일이 의도한 적용 후 내용이면 그대로 두고, 적용 전 hash·파일 식별자·수정 시각이 모두 일치할 때만 남은 쓰기를 수행한다. 어느 상태에도 맞지 않으면 `DEVELOPMENT_SOURCE_EFFECT_UNKNOWN`, 대조할 수 없는 복구 claim은 `DEVELOPMENT_SOURCE_RECOVERY_UNCERTAIN`으로 거부한다. 완료 기록과 실제 소스·이벤트·intent가 모두 맞아야 산출물로 받아들인다.

`verifyNativeDevelopment`와 `runManagedDevelopment`의 `recoverAfterFirstSourceWrite: true`는 `localNative: true`의 새 `retry-project` 과제에만 허용하는 공개 장애 검사다. 원래 MCP의 `write_source` 안에서 권한을 제한한 쓰기 자식 하나를 시작하고, 첫 파일을 쓴 직후 영수증 전에 의도적으로exit71로 종료한다. MCP는 직접 소유한 그 자식의 종료 결과와 intent의 PID를 대조한 뒤 이미 적용된 첫 파일을 확인하고 두 번째 파일만 쓴다. 다른 종료 코드·오류·timeout에는 복구를 시작하지 않는다. 자식은5초·입력8KiB·출력4KiB로 제한하고 기존 소스 검토·81개 판정·원장 한도를 유지한다. 두 조합의 로컬 native에서 해당 복구와 같은 세션의 다음 과제 완료를 확인했다. 복구 직전에도 처음 검토한 제안·intent hash가 같아야 하며, 각 중간 파일 집합도8KiB 한도 안에 있어야 한다.

같은 API의 `holdAfterFirstSourceWrite: true`는 새 로컬 `retry-project`의 첫 파일 쓰기 후 영수증 전에 MCP를 최대5초 대기시키는 별도의 공개 장애 검사다. 다른 중단·재개 옵션과 함께 사용할 수 없다. 이 경계에서 관리기를 종료한 뒤 새 관리기는 기존 소유권 검사로 native/MCP를 회수하고, 원래 부분 소스·intent hash·도구 기록 prefix·미관측 사용량을 v3 `partial-write` 중단 증거로 보존한다. 저장된 쓰기 소유자 PID가 더 이상 존재하지 않고 파일별 내용·식별자가 맞을 때만 남은 파일을 완성한다. 조회 오류·생존 PID·불명확한 효과나 복구 claim은 다른 종료/조회 경로로 우회하지 않는다. 첫 파일을 다시 쓰지 않고, 같은 세션의 finish 단계가81개 독립 판정을 통과한 뒤 다음 과제로 이어지는 것을 두 조합의 로컬 native에서 확인했다. 최초 실행은 `MANAGER_INTERRUPTED`·미완료로 남고 전체 예약을 유지한다. 완료된 복구와 계획 재조회는 추가 효과0이다. 이 증거는 고정된 첫 batch/첫 파일 중단 경계에 한정한다. 표식 없는 임의 중단·재개 기록 이후의 추가 중단·기록 잘림·PID 재사용 대응·전원 장애 내구성·실제 backend의 해당 장애 검증은 미완료다.

첫 복구 관리기가 중단되면 원래 복구 claim과 이벤트를 보존하고, `source-write-1-recovery-resume.json`에 이전 claim hash·새 소유자·이벤트 prefix의 hash/바이트 수를 결속한다. 원래 쓰기 소유자와 첫 복구 소유자가 모두 종료됐고, 고정 이벤트 순서와 파일별 효과가 대조될 때 한 번 재개한다. 이미 적용된 파일은 확인만 하며 미확정 재개 claim이 있으면 추가 쓰기를 거부한다. `prepareDevelopmentInterruption`의 로컬 공개 검사 옵션 `interruptPartialRecoveryAt`은 `claim`, `start`, `first-confirm`, `last-write`, `written`, `done` 경계에서 복구 관리기를exit74로 종료한다. 새 `holdAfterFirstSourceWrite` 과제에서만 허용한다. 두 조합의12개 실제 로컬 native 사례에서 관리기 종료→첫 복구 관리기 재종료→파일 효과 대조→같은session finish/다음 과제를 확인했다. 파일 완료 후 검토 기록 갱신 전에 종료된 경우에는 검토된 고정 공개 제안과 마지막 복구 소유자의 종료를 대조한 뒤 로컬 검토 기록을 완성한다. 이 동작은 실제 모드의 미검토 소스를 자동 승인하지 않는다. 원래 실패·사용량 예약과 최초 부분 소스 hash를 유지한다.

두 모델 설정의 새 공개 프로젝트에서 적용 전 소비자 검사 실패→제안 소스 적용→소비자8개/독립37개 검사 성공을 확인했다. 한 프로젝트는 관리기 종료 뒤 이미 끝난 첫 과제의 제안을 만들고 남은 과제를 이어 실행했다. 원장 잔액은 그대로이며 검사 재호출은 자식 시작0이다. 경로 이탈·기준/결과 hash 불일치·미적용·대상/제안/대상 지정 변경·검사 결과 유실·불일치한 정상 종료 영수증의9개 거부 사례도 통과했다. 검사 수의 변조를 별도로 확인하는 배포본 검사도 포함한다. 단일 파일 API는 검토한 순수 함수 모듈 한 개의 변경을 다룬다. 고정 판정기 성공은 전체 프로젝트 검사 성공이 아니므로 `wholeProjectVerified=false`·`longStageEvidence=false`를 유지한다. 이 공개 응답 fixture의 결과를 실제 모델 개발이나 필수 장기 단계 증거로 세지 않는다.

`long-stage-policy.mjs`는 4h→24h×3→72h 순서와 사건 수를 계산한다. 후보·ZIP·판정기·런타임·모델 조합이 같은지, 기본 context 설정, 각 단계의 시간·개발 수, 24h 세 실행의 압축/갱신 합계, 실패·개입·효과 중복·기록 누락·재사용된 개발 증거를 검사한다. 72h의 반복 병렬·취소·장애 복구는 각각 최소2회로 수량화했다. 입력한 관측치의 진위는 이 순수 계산기가 증명하지 않으므로, 모든 수량이 맞아도 `observationsIndependentlyVerified=false`와 `releaseVerdict=HOLD`를 반환한다. 실제 원자료를 대조하는 장기 관리기 연결과 시간 시험은 미완료다.

`auth-owner-protocol.mjs`는 기존 Codex 소유자와의 `initialize`·`account/read` 통신만 처리하는 검증용 경계다. 원문 계정 정보·진단을 반환하지 않고, 다른 home·외부 토큰 모드·도구/승인 요청·아직 보내지 않은 요청의 응답·중복·잘림을 거부한다. [공식 App Server 문서](https://learn.chatgpt.com/docs/app-server)와 설치0.154.0의 schema/help를 대조했다. 38개 순수 검사와 실제 공개 fixture 프로세스9개가 통과했으며, RPC 갱신 응답만으로 정상 token 갱신을 확인했다고 표시하지 않는다. `verify-auth-owner-read.mjs --existing-owner-read-only`는 설치 CLI의 기존 proxy만 사용하며 daemon/thread/refresh/login/config 명령을 시작하지 않는다. 현재 머신의 첫 연결은 초기화 응답 없이exit1로 종료됐고 proxy 회수는 확인했다. 원문 진단을 보존하지 않아 상세 원인은 미확정이며, 실제 소유자 결속·정상 갱신 및 제품 supplier 연결은 미완료다.

Not verified / 제한:

- astra 최초 공개 단문 검사에서 전달 전 upstream error 1건이 있었고 이후 단독 검사는 성공했다. 최초 오류의 원인은 미확정이며 외부 서버 무장애를 보장하지 않는다.
- 기본 400K 창·320K 자동 압축 목표는 설정 계약이다. 기본값에서의 실제 발동·압축 후 전체 이력 보존은 미검증이며, 축소 창에서의 기존 실측과 구분한다.
- 정상 인증 갱신·만료 경계, 모든 사용자 hook/plugin·permission/plan UI 조합, 프롬프트 캐시 실제 적중은 미검증이다. 인증 파일의 직접 편집이나 계정 전환으로 시험을 대신하지 않는다.
- 두 조합 각각의 4시간 → 24시간 3회 → 72시간 단계는 시작하지 않았다. 기본 메인·자식 압축, 정상 인증 갱신과 원래 개발 과제 완료의 필수 사건 수를 모두 채워야 하며 짧은 기능 검사로 대체하지 않는다.
- native Bash 명령의 허용·거부 경계는 새 공개 fixture에서 확인했다. `verify-native-permissions.mjs --local-native sol <pwsh.exe 경로>` 및 luna 설정 각각에서 허용 worker 시작/효과1회, 명시적 거부 도구 결과, 거부 worker 시작/효과0회와 종료 상태를 대조했다. 실제 native와 loopback 응답을 사용하며 모델·credential 호출은0이다. runtime 제한·native 권한 검사·기존 hooks는 유지한다. 이 결과는 해당 명령 규칙의 증거이며 hooks·MCP·UI의 모든 권한 조합을 입증하지 않는다.
- 취소·등록 교체·형제 격리의 합성 검사는 통과했지만 UI 취소 시점까지 연결한 전체 실측은 조건부다. 동적 symlink/junction 검사는 정책 차단으로 실행하지 않았다.
- 단일 completed 알림 자동 복귀에는 기존 실측이 있다. 연속된 독립 completed 알림은 최대64개를 모두 검증한 뒤 완료 증거를 함께 소비하도록 보완했으며, 앞 자식 증거의 재사용·중복·위조·중간 취소에 대한 합성 검사를 통과했다. 이번 `-p`의 foreground/background/fork 비교에서는 자식 알림이 부모에 도착하지 않아 실패했다. 별도 `CompletionMode relay` 검증은 대기하는 메인 응답을 끝내 다음 native 알림을 받고, 두 자식 완료를 확인한 뒤 정확한 부모에게 SendMessage1회와 TaskOutput1회로 이어진다. 두 조합에서 같은 부모의 verified-resume·최종 완료·정리를 확인했다. 이 결과는 메인이 조정한 재개의 증거이며, 직접 native 다중·실패·취소 알림 자동 복귀는 미완료다. Workflow는 inline 신규 실행과 이미 완료된 캐시 재사용의 증거가 있으며, 캐시 미적중 resume·중첩·custom agentType은 미완료다.
- Anthropic 서버 전용 기능, 비스트리밍 API, 서버 실행 도구·첨부/PDF·미지원 context edit·sampling 필드는 지원하지 않는다. 신규 native 버전과 Codex 비공개 backend 변경은 재검증이 필요하다. `CLI_VERSION_UNVERIFIED` 안내를 숨기지 않는다.

## 검증과 무결성

```powershell
pwsh -NoProfile -NonInteractive -File src/run-node-tests.ps1 -Root . -TestFiles 'src/test-*.mjs'
pwsh -NoProfile -NonInteractive -File verification/verify-native-headless.ps1 -Live -Case text -Model luna -Effort max
pwsh -NoProfile -NonInteractive -File verification/verify-native-headless.ps1 -Live -Case workflow -Model sol -Effort low
pwsh -NoProfile -NonInteractive -File verification/verify-native-headless.ps1 -Live -Case completion -CompletionMode relay -Model luna -Effort max -TimeoutSeconds 120 -RequestLimit 16
```

첫 명령은 검토된 로컬 회귀이며 실제 backend를 호출하지 않는다. 기본 러너의 `review-diff`는 PATH 부재로 notRun을 표시한다. 별도 검토된 Git 경로의 제한 환경에서는 실제 검사가 통과했다. native Workflow와 symlink의 합성 suite notRun을 실검사 성공으로 세지 않는다.

Workflow의 긴 기록은 작은 조각으로 순차 읽고 해당 자식의 출처를 끝까지 검증한다. 단일 journal 레코드128KiB, snapshot16MiB·65536레코드와 선택 전체1초의 읽기 작업 한도가 있으며, 중복·실패 기록과 재검증 사이의 기존 기록 변경은 거부한다. 자식 transcript는 최대1MiB의 첫 레코드로 신원을 검증한다. 긴 journal·transcript 및 위조·잘림·취소에 대한 로컬 검사를 통과했지만, 실제 긴 Workflow의 cache-miss resume는 아직 미완료다.

`-Live` 검사는 기존 로그인으로 공개 고정 fixture를 실제 전송하며 사용량이 발생한다. 새 작업 전용 프로필을 사용하고 프로세스당 최대 120초로 제한한다. 사용자 대화·프로필을 복사하지 않는다. `failure-resume`은 전달 후 오류를 의도적으로 1회 주입해 보존·복구를 확인하는 별도 사례다.

ZIP 옆 `manifest.json`은 소스 commit, 파일별 크기·SHA256, ZIP SHA256을 기록한다. SHA256은 손상 검사용이며 서명을 대신하지 않는다. 코드·검사·이 안내만 묶고 사용자 상태·프로필·Git 이력·감사 기록·과거 프롬프트·생성된 schema는 포함하지 않는다. 원본 Git checkout에서는 `pwsh -NoProfile -NonInteractive -File verification/build-release.ps1`로 같은 commit의 ZIP을 다시 만들 수 있다. 압축을 푼 배포본에는 Git 이력이 없어 빌더를 실행하지 않는다. 개발 fixture 검증기는 처음 실행할 때 프로젝트의 `.tmp` 디렉터리를 생성하므로 회귀 검사를 먼저 실행할 필요가 없다. 기존 파일은 보존하며 `.tmp`가 일반 파일이나 링크이면 덮어쓰지 않고 거부한다. 디렉터리가 없던 배포본의 `ENOENT` 실패를 재현한 뒤 초기화·기존 작업 보존·일반 파일 충돌을 실제 공개 자식 프로세스로 검사했다. 동적 링크 검사는 기존 정책 차단 상태다.

빈 keepalive 처리의 근거는 [공식 Codex SSE 소비자](https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/sse/responses.rs)의 선택 필드와 미처리 이벤트 분기다(2026-09-13 확인). 이 구현에서 도출한 최소 호환 범위로, type만 있는 keepalive를 응답 시작 후·완료 전에만 출력 없이 처리한다. 추가 필드·순서 위반·완료 응답 누락을 거부하고 도구는 최종 검증 후에만 전달한다. 원본 JSON의 중복 type 키가 사라지는 문제도 실제 loopback으로 재현하여 빈 envelope를 전달하기 전에 거부한다. 정상 공백/Unicode escape는 유지한다. 이 변경은 모든 미지 이벤트나 원래 private backend payload의 호환성을 증명하지 않는다. 새 keepaliveEvents는 검증을 통과한 수만 기록한다.

2026-09-13 최신 관련 로컬 검사는 parser19개, raw SSE8개, 진단17개와 기존 protocol이 통과했다. 공개 heartbeat를 넣은 실제 local-native 두 과제는 10개 이벤트를 처리하고 동일 세션·oracle·원장·종료를 대조했다. 별도 전체 gateway 검사는20초 한도에서 실패했고 변경 전4ec7c54 ZIP에서도 재현됐다. 같은 응답의 클라이언트 종료 비교는 두 방식 모두 약1초 fallback을 관측했으며 이 실패를 수정하거나 합격 처리하지 않았다.
