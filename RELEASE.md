# Clauduct — Windows 릴리즈 안내

현재 후보의 무인 개발 출하 판정은 **HOLD**다. 로컬 ZIP 생성은 파일 구성·무결성 검사이며 전체 출하 검증의 통과를 뜻하지 않는다. 2026-09-13 후보에서는 정상 HTTP 일부가 재성공했지만 request-inspector, user-session watchdog, TCP 반닫기 검사에 실패가 남아 있다. 취소 검사의 metadata 준비 경쟁은150ms 지연으로 재현하고, 실제 취소 도착까지 준비 완료를 보류하는 fixture로 고쳐 관련5개 파일 회귀를 통과했다. 같은 loopback 반닫기 이상은 Clauduct·Node 없는 .NET 비교에서도 관측했으며 원인과 정상 환경에서의 재검증은 미완료다.

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
- PowerShell `7+`는 검증 스크립트에 필요하다. 별도 npm 의존성 설치는 없다.

Claude Code의 UI·로컬 도구·기존 권한 검사는 유지하고 모델 요청만 `127.0.0.1` gateway를 거쳐 ChatGPT Codex backend로 보낸다. Codex app-server 경로가 아니다. provider/secret 계열 환경 변수는 native 자식에 전달하지 않는다. 이 환경 변수에 의존하는 사용자 도구·MCP는 별도 호환성 확인이 필요하다.

## 동작과 오류 계약

- 기본 메인은 `astra/low`다. 명시 모델의 기본 effort는 astra/medium, sol/xhigh, terra/high, luna/max이며 `--effort`가 우선한다.
- `-p`/`--print`는 비대화형이다. stdout은 native 결과만, wrapper 안내·종료 상태는 stderr다. 명시적 print 없이 파이프로 실행하면 TTY 오류로 종료한다.
- 텍스트는 스트리밍하고 도구 호출은 정상 완료 검증 뒤 전달한다. 전달 전 일시 I/O·429·5xx는 제한 재시도하며 전달 후 오류는 자동 재실행하지 않는다. 완료된 도구를 반복하지 말고 `--resume`으로 이력을 이어간다.
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

`-Case text -FailFirstConnection` 검증에서는 첫 연결 전에 DNS 오류를 한 번 주입한다. `luna/max`와 `sol/low` 각각에서2시도 중 실제 모델 응답1개로 원래 요청을 완료했고, 첫 실패 분류·이후 성공·원장·정리를 확인했다. 연결 오류 주입의 로컬51개 검사는 모델과 검색의 TLS/접근 거부 시도1회, DNS 재시도 한도와 고정 진단을 검사했다. 실제 자체 서명 TLS 서버를 사용하는 별도 시험은 준비 단계에서 종료되어 아직 미검증이다.

`verify-native-service-recovery.mjs --local-native`는 실제 native와 loopback HTTP를 사용하고 모델 응답은 고정 공개 fixture로 공급한다. 두 조합 설정에서 반복503, HTTP200 error, 텍스트 전달 후 단절·잘못된 UTF-8·순서 오류의10개 사례를 확인했다. 실패한 응답의 미완료 도구는 실행되지 않았고, 오류 종료 뒤 관리기가 같은 세션을 재개하여 영수증 조회와 원래 보고서를 완료했다. 효과 호출·조회·보고서 호출은 각각1회이며 첫 실패와 최종 성공을 따로 보존한다. 이 검사는 실제 backend 장애나 장기간 서비스 대기·인증 복구의 증거를 대신하지 않는다. fixture의 선택적 `audit` 모드는 도구 이름만 효과 전에 기록하고, 기록 실패 시 효과를 실행하지 않는다.

공용 검증 관리기의 `WAITING`은 검증된429/5xx 분류와 재개 시각을 실패 exitCode와 같은 동기 원장 행에 보존한다. 새 프로세스에서도 기한 전에는 worker를 시작하지 않으며 이후 기록으로 서버의 더 늦은 기한을 줄이지 않는다. `deferred-503` 사례에서는 두 조합 설정의 실제 native가503+Retry-After60을 받고, 새 관리기의 이른 재개는worker0으로 대기한 뒤 같은 세션에서 원래 보고서를 완료했다. 이 두 시험은 실제 시간을 각각60초 이상 기다린 로컬 서비스 시험이며 모델 전송·credential 조회는0이다. 기록 완료 후 결과 출력 전 프로세스 종료, 잘못된 상태·타입·분류와 미확정 효과의 재실행 거부도 검사했다. 응답을 받았지만 아직 대기를 기록하지 못한 crash 구간과 전원 손실은 미검증이다.

`verify-native-recovery.mjs --live <model> <pwsh.exe> <prior-attempts> <prior-elapsed-ms> --service-signal`은 기존 로그인으로 실제 모델을 호출하는 별도 검사다. `luna/max`와 `sol/low`에서 효과 영수증 뒤 다음 전송 직전에 합성503 분류를 주입하고, 같은 세션을 자동 재개해 상태 조회와 원래 보고서를 완료했다. 각 조합은 실제 backend4요청, 효과·조회·보고서 각각1회였으며 오류 종료와 최종 성공, 지정 라우팅, 숫자 원장, 프로세스 회수를 대조했다. 주입된 신호는 `backendAttempted=false`로 기록하고 실제 backend HTTP503 응답이나 요청 사용량으로 세지 않는다. 도구 가드는 각 단계의 고정 도구와 빈 인수만 허용한다. 이 짧은 검사는 장기 서비스 중단이나 임의 개발 도구의 복구를 대신하지 않는다.

`native-output.mjs` 수집기는 JSON/stream-json을 행마다 검사하고 중간 메시지를 누적하지 않는다. 두pipe의 정상 EOF, 유일한result/종료상태, native exit와 독립oracle이 일치해야 완료다. 실제 로컬 자식6개의 느린 소비·32MiB 이상 중간 출력·큰result·과대result·pipe 단절·잘림을 검사했으며, 효과와보고서가 있어도 결과가 누락되면완료=false였다. 소규모 복구 검증기에는 총1MiB·한행1MiB·4096레코드 상한으로 연결했다. 두 조합 설정의 실제 native stream-json에서도 전달 후 장애·오류result·동일 세션 재개·최종result와cleanup을 확인했다. 실제 모델 호출을 쓰는 복구 경로의 앞선 결합 성공은 수집기 교체 전 후보의 증거다. 장기 관리기 전체·native 최종result 직전pipe 단절 후 개발 복구는 아직 미검증이다.

`verify-native-task-isolation.mjs --local-native <model> <alpha|beta> <pwsh.exe>`는 두 고정 background worker 중 지정한 하나만 취소하는 검사다. `luna/max`와 `sol/low` 설정에서 취소 대상을 바꾼 네 사례가 통과했다. 실제 native의 Bash 결과와 call ID를 대조해 TaskStop 대상을 고정하고, 대상 PID 종료 후에도 다른 worker가 살아 있으며 TaskOutput으로 정상 완료되는지 확인했다. 두 worker의 제한된 파일 권한, 정확한 도구 호출, result/상태 각1개와 회수를 함께 검사했다. native가 background ID를 worker의 첫 기록보다 먼저 반환하므로, 아직 없거나 작성 중인 시작 기록만 최대1500ms 기다린다. 이 시험의 모델 응답은 로컬 공개 fixture이며 실제 backend·credential 사용은0이다. 모델을 실행하는 Agent 자식의 취소와 늦은 upstream 응답 격리까지 검증한 것은 아니다.

추가 회귀의 `test-cancel-snapshot.mjs`에서15초 timeout이 한 번 관측됐다. 단계 표시를 넣은 별도 관측본은 기존6개 검사를 통과했지만 원래 실패의 원인은 미확정이다. 원본 검사에도 고정 단계명과 완료 검사 수를 timeout 진단에 추가했으며, 시간 한도와 기존 assertions는 유지했다.

Not verified / 제한:

- astra 최초 공개 단문 검사에서 전달 전 upstream error 1건이 있었고 이후 단독 검사는 성공했다. 최초 오류의 원인은 미확정이며 외부 서버 무장애를 보장하지 않는다.
- 기본 400K 창·320K 자동 압축 목표는 설정 계약이다. 기본값에서의 실제 발동·압축 후 전체 이력 보존은 미검증이며, 축소 창에서의 기존 실측과 구분한다.
- 정상 인증 갱신·만료 경계, 모든 사용자 hook/plugin·permission/plan UI 조합, 프롬프트 캐시 실제 적중은 미검증이다. 인증 파일의 직접 편집이나 계정 전환으로 시험을 대신하지 않는다.
- 두 조합 각각의 4시간 → 24시간 3회 → 72시간 단계는 시작하지 않았다. 기본 메인·자식 압축, 정상 인증 갱신과 원래 개발 과제 완료의 필수 사건 수를 모두 채워야 하며 짧은 기능 검사로 대체하지 않는다.
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

ZIP 옆 `manifest.json`은 소스 commit, 파일별 크기·SHA256, ZIP SHA256을 기록한다. SHA256은 손상 검사용이며 서명을 대신하지 않는다. 코드·검사·이 안내만 묶고 사용자 상태·프로필·Git 이력·감사 기록·과거 프롬프트·생성된 schema는 포함하지 않는다. 원본 Git checkout에서는 `pwsh -NoProfile -NonInteractive -File verification/build-release.ps1`로 같은 commit의 ZIP을 다시 만들 수 있다. 압축을 푼 배포본에는 Git 이력이 없어 빌더를 실행하지 않는다.
