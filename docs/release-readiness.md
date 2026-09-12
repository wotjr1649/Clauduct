# Clauduct 릴리즈 검증

2026-09-12 시작. 기준 커밋 `6c263c0`, 개발 브랜치 `release/stability-2026-09-12`.

사용자 목표는 Anthropic 서버 전용 기능을 제외한 약속된 기능의 안정적인 사용과 비대화형 실사용 검증이다. 이 문서는 출하 증거를 기록하며, 기존 감사의 과거 관측을 새 실행 결과로 바꾸지 않는다.

## 출하 조건과 진행

1. [x] 완료 알림 복귀의 지원 범위·복원 불가 원인·사용자 UI 취소 관측을 현재 상태표에 분리했다.
2. [x] 제공 기능과 실제 구현을 전수 대조하고 재현된 결함을 수정했다. 텍스트·도구·모델/effort·자식/Workflow·취소/재개·검색·이미지·로컬 MCP·컨텍스트·진단을 포함하며, 미검증·차단 범위는 아래와 현행 상태표에 유지한다.
3. [x] 비대화형 실행으로 정상 작업 및 실패 뒤 결과 보존·도구 미중복·종료 정리를 검증했다. 로컬 합성과 실제 backend 증거를 구분한다.
4. [ ] 릴리즈 구성의 전체 회귀, 배포 산출물 내용·경로·시작/종료 검사와 최종 판정을 기록한다.

외부 서비스 무장애나 모든 future client 버전 호환성은 입증할 수 없다. 복구 가능한 전달 전 장애는 제한된 재시도로 처리하고, 전달 후 장애·취소는 미완료 상태와 진단을 보존해야 한다. 실패한 응답의 도구를 실행하거나 이미 끝난 작업을 자동 중복 실행하면 불합격이다.

## 기준선

Verified: 사용자 작업 루트에서 다음 명령을 실행했다. 파일 20/20, 실패 0, 종료 코드 0, 16.74초. 검사 전후 tracked·staged 변경 없음과 기존 untracked 목록 동일을 확인했다.

```powershell
pwsh -NoProfile -NonInteractive -File src/run-node-tests.ps1 -Root D:/AIDEV/Clauduct -TestFiles 'src/test-*.mjs'
```

Not verified: 파일 단위 성공 안에 `review-diff`(PATH 부재/ENOENT), native Workflow 실행, completion·Workflow symlink 검사의 `notRun`이 있다. 이들은 실검사 성공으로 세지 않는다.

작업 트리는 `D:/AIDEV/Clauduct/.tmp/release-2026-09-12`다. 기존 사용자 파일을 포함하지 않고 변경 파일을 개별 이름으로 stage한다. 원래 작업 루트로 통합하기 전 최종 변경과 검증 결과를 확인한다.

## 출하 판정

진행 중. 릴리즈 통과나 실제 backend 무오류를 아직 선언하지 않는다.

## 비대화형 실행 보완

기존에는 native에 `-p`를 전달해도 wrapper가 TTY를 요구했다. 명시적 `-p`/`--print`를 식별해 파이프 실행을 허용하고, wrapper 출력은 stderr로 보내 native JSON/stream-json stdout을 보존한다. 기본 대화형 진입점과 과거 PoC의 SEND 절차는 유지한다. 디버그·임의 Node 실행 인수·proxy/TLS 우회·다른 Codex 홈·계정 변경 검사를 제거하지 않았다.

Verified: 새 `test-headless`가 수정 전 print 모드 미인식 assertion으로 실패했다. 수정 후 19개 검사 통과: 두 flag, 값/구분자 혼동 방지, 명시적 비대화형 진입, runtime·인증 경계의 기존 거부, dry-run, 실제 합성 Node 자식의 성공/실패 JSON과 stderr 진단·9개 정리 플래그다. `src/test-*.mjs` 전체 21/21, 종료 코드 0(17.99초). 공유 진입점 회귀 `poc/test-user-session.mjs` 89/89, loopback 72회, 종료 코드 0이다.

Not verified: 이 결과는 실제 Claude 또는 backend 실행 결과가 아니다. 기준선의 세 종류 `notRun`은 그대로 남아 있다.

공식 근거: [Claude 비대화형 실행](https://code.claude.com/docs/en/headless)은 `-p`, JSON/stream-json, 오류 시 비정상 종료 코드를 정의한다. [OpenAI 스트리밍](https://developers.openai.com/api/docs/guides/streaming-responses)은 text delta·정상 완료·error를 별도 이벤트로 정의한다. 공개 Responses 문서가 비공개 Codex backend의 모든 동작을 보증하지는 않는다.

## 실제 backend — 공개 고정 문장

Verified: `node verification/verify-live.mjs --live`를 비대화형으로 실행했다. `gpt-5.6-luna/low`, clientVersion `0.154.0`, HTTP 200, upstream 1회, 요청 성공 1·실패 0, 정확한 고정 답변·완료 프레임·자원 정리를 확인했다. 종료 코드 0이다. 인증 값·응답 본문·기존 대화·저장소 내용을 출력하거나 검증 기록에 복제하지 않았다.

이 검사는 실제 credential supplier·HTTPS transport·gateway 변환을 사용하고, 새 요청의 본문은 코드에 고정된 공개 문장뿐이다. 도구가 없고 재시도하지 않으며 60초 안에 취소한다. 검증기 자체의 정상·틀린 답변·실패 보고 검사는 합성 loopback 3개로 별도 통과했다.

Not verified: 실제 Claude 실행은 0회다. 이 결과로 native 도구·자식·UI와의 전체 왕복을 통과로 표시하지 않는다. reference `0.153.4`에 대한 `CLI_VERSION_UNVERIFIED` 안내는 유지된다.

## 실제 native 비대화형 왕복

`verification/verify-native-headless.ps1 -Live`는 새 작업 전용 `CLAUDE_CONFIG_DIR`와 작업 디렉터리를 만들고 native Claude `2.1.269` 및 실제 backend를 사용한다. 개인 대화·프로필·플러그인을 복사하지 않고 관리형 정책·native 권한 검사는 유지한다. 시간 제한은 프로세스마다 기본 90초, 최대 120초다. 원문 stdout/stderr는 저장하거나 출력하지 않고 아래 판정만 보고한다. 세션·도구 증거가 필요한 경우 새 프로필에 native가 기록한 공개 fixture 이력만 로컬 검증한다.

| 실행 | 관측 |
|---|---|
| `-Case text` | 정상 JSON stdout, 정확한 고정 답변, 요청 성공 1·실패 0, 정리 9개 true, exit 0 |
| `-Case stream-json` | 실제 native partial `stream_event`의 text delta가 정확한 답변을 이루며 최종 result JSON은 1개. stdout의 모든 줄을 JSON으로 파싱, 정상 종료·정리·exit 0 |
| `-Case read-edit` | 실제 Read→Edit로 새 `math.mjs`의 빼기를 덧셈으로 수정. 전체 파일이 기대한 한 줄과 일치, 정확한 최종 답변·정상 요청 결과·정리·exit 0 |
| `-Case agent` | 실제 `clauduct-probe-luna`의 Read와 부모 복귀. 자식의 `selectionSource=definition-model`, `model=gpt-5.6-luna`, `effort=max`를 종료 진단에서 확인. 정상 최종 답변·요청 결과·정리·exit 0 |
| `-Case workflow` | 정확한 inline script의 Workflow 1회·TaskOutput 1회. native journal의 자식 started/result 각 1개와 `sum=5`, `workflow-result`·`workflow-subagent`·`luna/low` 라우팅, 부모 최종 답변·정리·exit 0 |
| `-Case mcp` | 프로젝트 `.mcp.json`으로 발견한 stdio 계산 도구가 `a=2,b=3`으로 정확히 1회 호출됨. 결과 5 이후 정확한 최종 답변·정상 요청 결과·정리·exit 0 |
| `-Case resume` | 서로 다른 두 프로세스의 session ID 일치. 첫 공개 코드 저장과 다음 프로세스의 정확한 코드 회상, JSON 결과·정리·exit 0 |
| `-Case failure-resume` | 실제 MCP 호출 1회 성공 → 다음 응답의 내용 전달 뒤 고정 upstream 오류 1회 주입 → 명시적 `--resume`에서 기존 호출 기록 Read. 실패 프로세스 exit 1·`is_error=true`, 전달 후 재시도 0, 실패 이력 보존. 재개 exit 0. 모든 단계의 session ID 일치·정리 9개 true, MCP 호출 총수는 끝까지 1 |
| `-Case image` | 실제 Read 1회, PNG image tool_result 1개, backend의 정확한 색상 답변. 정상 요청 결과·JSON·정리·exit 0 |
| `-Case webfetch` | 실제 `https://example.com` WebFetch 1회, 정상 tool_result와 정확한 페이지 제목. 정상 요청 결과·JSON·정리·exit 0 |
| `-Case websearch` | 실제 WebSearch 1회, `webSearchRequests=1`, `webSearchCalls=1`, 링크 수 >0, 정상 tool_result·최종 답변·정리·exit 0 |
| `-Case build` | native Bash에서 고정된 `node verify.mjs`를 foreground로 정확히 1회 실행. 실제 Node assertion 성공과 도구 결과·최종 답변·정리·exit 0 |
| `-Case build-powershell` | 새 검증 프로필의 공식 `CLAUDE_CODE_USE_POWERSHELL_TOOL=1` opt-in 뒤 실제 PowerShell 도구 1회·Node assertion·최종 답변·정리·exit 0. `CLAUDE_CODE_POWERSHELL_RESPECT_EXECUTION_POLICY=1` 유지 |
| `-Case background` | 실제 Bash `run_in_background=true` 1회와 TaskOutput 1회. worker의 시작·완료 각 1개, 결과 회수·최종 답변·정리·exit 0 |
| `-Case cancel-task` | 실제 background Bash 1회와 TaskStop 1회. 시작 기록 1개·완료 기록 없음, 기록된 worker PID가 OS에서 종료됨을 확인. 정상 요청 결과·최종 답변·정리·exit 0 |
| `-Case text -Model astra` (`sol`, `terra`도 각각 실행) | 세 모델 모두 정확한 고정 답변, 실제 요청의 모델·low effort, JSON·정리·exit 0을 확인. astra 최초 실패는 아래 별도 보존 |

agent 첫 시도는 검증기가 `--tools Agent,TaskOutput`으로 Read를 제외해 native가 자식 생성을 거부했다. 프로세스는 exit 0이었지만 검증기는 답변·라우팅 미달로 실패를 반환했다. 원문 tool_result의 `zero tools`·`unrecognized [Read]`를 확인한 뒤 검증기의 도구 목록에 Read를 포함했고 다음 시도가 통과했다. gateway/API 실패나 제품 수정으로 분류하지 않는다.

Workflow는 `-p`에서 명시적 도구 호출과 해당 호출의 정상 allow rule을 사용했다. `ultracode` 키워드나 human origin 위장은 사용하지 않았다. 한 자식의 공개 산술 문제만 실행하며 script의 filesystem·shell 접근은 없다. `CLAUDE_CODE_DISABLE_WORKFLOWS`가 있으면 보존한다. [공식 Workflow 문서](https://code.claude.com/docs/en/workflows)는 비대화형 명시 호출과 키워드 활성화 경로를 구분한다.

실패 복구 검증기의 첫 시도는 마지막 가변 길이 옵션이 프롬프트를 소비해 모델 요청이 0회였다. 프롬프트 앞에 `--`를 넣어 해결했다. 다음 시도에서 아래의 과거 도구 이력 거부를 재현했으며, 제품 수정 후 3단계가 모두 통과했다. 주입 오류는 정상 장애를 숨기는 설정이 아니라 명시적 `--verify-fallback blocked` 검증 경로이며 일반 실행에는 적용되지 않는다.

## 실제 왕복에서 발견한 결과 보존 결함

1. **빈 headless JSON 결과**: 실제 세션에는 정확한 답변이 있으나 마지막 assistant 블록이 `redacted_thinking`이어서 JSON `result`가 길이 0이었다. 두 실행에서 재현했다. 텍스트 delta와 content index는 유지하고 텍스트의 `content_block_stop`만 opaque reasoning 뒤·검증된 도구 앞에 전달한다. reasoning이나 텍스트를 삭제·복제하지 않는다. 첫 블록을 읽는 WebFetch의 Message 구조도 그대로다. 로컬 회귀를 먼저 실패시킨 뒤 수정했으며 실제 `resume`의 두 프로세스에서 정확한 JSON 결과와 이력 복원을 확인했다. [Messages 스트리밍 명세](https://platform.claude.com/docs/en/build-with-claude/streaming)의 block index에 맞춰 전달하며 현재 native 실측을 증거로 삼는다.
2. **도구 목록 축소 후 재개 실패**: MCP 호출이 끝난 세션에서 도구를 비활성화해 재개하면 과거 `tool_use`까지 현재 정의 목록으로 검사해 `INVALID_TOOL_CALL`로 거부했다. 완료된 이력의 호출·참조는 데이터로 유지하며 현재 `tools`/`names`는 늘리지 않는다. 새로 생성한 비활성 도구 호출, 없는/중복 결과, 잘못된 식별자는 계속 거부한다. 회귀의 수정 전 실패·수정 후 성공과 실제 `failure-resume`에서 MCP 재활성화·중복 없이 복구되는 것을 확인했다.
3. **진단 예외 본문 노출**: 상태 projection 예외의 임의 메시지가 종료 출력·기록으로 전달되는 경로를 합성 private marker로 재현했다. 고정 `STATUS_UNAVAILABLE`만 기록하도록 수정한 뒤 chat 28/28 통과했다.
4. **delta 없는 빈 텍스트 완료**: PowerShell 검증 중 `SNAPSHOT_MISMATCH`가 두 번 발생했다. 두 번째 실행에서 새 고정 라벨 `snapshotMismatchEvent=response.output_text.done`과 downstream 전달 없음을 확인했다. 빈 텍스트의 done 이벤트에는 delta가 없어도 되는데 parser가 앞선 delta를 필수로 요구했다. 같은 형태의 로컬 검사를 먼저 실패시킨 뒤 빈 문자열만 기존 text 처리 경로로 초기화했다. delta가 없는 비어 있지 않은 텍스트는 계속 거부한다. 이후 실제 요청은 정상 완료됐다.

PowerShell의 중간 시도는 답변이 맞고 exit 0이어도 도구 호출이 0회여서 검증 실패였다. 공식 opt-in을 새 검증 프로필에만 적용한 다음 실제 PowerShell 1회 호출을 확인했다. 모델의 성공 주장만으로 검증을 통과시키지 않는다. [native 도구 참조](https://code.claude.com/docs/en/tools-reference)의 PowerShell 설정·TaskOutput·TaskStop 의미를 기준으로 삼았다.

Verified: 빈 텍스트·불일치 이벤트 진단까지 반영한 `src/test-*.mjs` 23/23, exit 0, 17.87초. `verification/test-manual-http-probe.mjs` 88/88도 통과했다. `test-review-diff.mjs`는 검토한 Git 실행 파일 디렉터리만 PATH에 넣은 별도 Node permission 실행으로 실제 통과했다. 기존 러너의 환경과 symlink 제한은 변경하지 않았고 `--allow-child-process`의 SecurityWarning도 숨기지 않았다. 새 MCP fixture의 자체 검사는 4/4이며 수정한 현행 문서 4개의 상대 링크 60개는 모두 존재한다.

Not verified: 이 결과를 강제 창 종료, 실제 계정 회전, 모든 향후 native 버전이나 기본 320K 자동 압축의 증거로 확대하지 않는다. 기준선의 symlink 차단은 유지한다.

### 실제 astra 실패와 이후 성공

세 모델의 공개 단문 검사를 함께 실행했을 때 astra/low 1건이 HTTP 200의 `error` 이벤트로 실패했다. `UPSTREAM_ERROR_EVENT`, upstream code/type은 모두 `OTHER`, downstream 전달 없음, 시도 1회, 재시도 0, 정리 9개 true였다. 모델을 바꾸지 않고 다른 검사 종료 후 단독 실행한 astra/low는 정확한 답변·정상 JSON·exit 0으로 끝났다. `sol/low`와 `terra/low`는 첫 실행에서 통과했다.

Not verified: 최초 astra 오류의 원문은 보존하지 않았으며 동시성·quota·서버 상태 중 무엇이 원인인지 확정하지 않는다. 이후 성공을 그 오류의 수정 증거로 사용하지 않는다. 알 수 없는 실패를 무조건 재시도하거나 모델을 몰래 전환하지 않았다.

## 설치 위치와 사용자 설정

Verified: 별도 작업 트리에서 `TASK_ROOT`가 원래 개발 폴더를 가리키는 assertion 실패를 재현했다. fixture 경로를 현재 설치 위치로 계산하고, 실행 파일·인증 홈의 개인 사용자명 고정값을 OS 사용자 홈 기준으로 바꿨다. 임의 `USERPROFILE` 값으로 credential home을 바꾸지 않는다. [Node.js `os.userInfo`](https://nodejs.org/download/release/v24.1.0/docs/api/os.html#osuserinfooptions)의 OS 제공 홈을 사용한다.

개인 statusline 스크립트의 강제 지정은 제거했다. native의 사용자 설정이 statusLine을 결정하며 사용자 설정 파일은 수정하지 않는다. 경로·환경 주입 검사는 7개 통과, 변경 후 `src` 23/23와 PoC 6/6가 통과했다. PoC 세부 결과는 adapter 318, inspection 14, Read 33, gateway 74, request-inspector 90, user-session 89다.

## 로컬 배포 산출물 검증

`verification/build-release.ps1`은 tracked 변경이 없는 정확한 Git checkout에서 커밋된 68개 파일만 ZIP으로 묶는다. `src` 실행 코드·검사, 필요한 `poc` 모듈·검사, 실제 검증기, [설치 안내](../RELEASE.md), 옵션 분류를 포함한다. 사용자 상태·프로필·감사 기록·과거 프롬프트·생성 schema·Git 이력과 모든 untracked 파일은 제외한다. 파일 경로·타입·개수·크기를 확인하고 파일별 및 ZIP SHA256을 `manifest.json`에 기록한다. 외부 게시·push·계정/전역 설치는 수행하지 않는다.

Verified: `c310c59`의 ZIP을 두 번 만들어 동일 SHA256을 확인했다. 공백이 있는 새 설치 경로에 풀고 68개 파일의 크기·해시를 전부 대조했다. 실제 `clauduct.cmd --dry-run -p`에서 인증 조회 0·자식 실행 없음과 print 구성을 확인했다. 배포본의 `src` 23/23(17.73초), PoC 6/6(12.81초), 별도 제한 환경의 review-diff가 통과했다. 같은 배포본에서 실제 Workflow와 failure-resume 3단계도 모두 통과했으며, 오류 프로세스의 exit 1·실패 이력·cleanup과 재개 시 MCP 호출 총 1회를 다시 확인했다.

추가 러너 검사의 첫 실행은 배포본에 포함하지 않은 개발용 `README.md`를 전제로 해서 실패했다. 런타임의 경로 검사는 이를 허용하지 않고 `TEST_FILE_NOT_FOUND`로 거부한 상태였다. 존재하는 `.ps1` 거부를 검사하도록 fixture를 `src/test-run-node-tests.ps1`로 바꿨다. 소스 checkout에서 환경 격리 3/3·경로 거부 3/3·없는 루트·실패 exit·timeout 후 프로세스 트리 종료가 통과했다. 같은 검사와 최종 ZIP은 수정 후 별도로 확인한다.
