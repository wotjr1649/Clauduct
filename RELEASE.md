# Clauduct — Windows 릴리즈 안내

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
- JSON `is_error`, 종료 코드, `CLAUDUCT_REQUEST_STATUS`의 `requestOutcome`·`failureHistory`·`cleanup`을 함께 확인한다. exit 0이나 `Clauduct 종료: SUCCESS`만으로 세션 중 모든 요청이 성공했다고 판단하지 않는다.
- 종료 상태는 설치 폴더의 `.clauduct-status/request-status.jsonl`에도 추가된다. 원문 오류·인증 값은 기록하지 않는다. 기록 실패는 명시적으로 안내하며, 강제 프로세스 종료에서는 기록을 보장하지 않는다.
- 비대화형 자동화에는 `--max-turns`와 호출자의 시간 제한을 둔다. `--bg`/`--background`의 세션 분리 동작은 이 릴리즈에서 실검증되지 않았으며, 검증된 background 도구/TaskOutput/TaskStop과 구분한다.

[CLI 옵션 경계](docs/claude-option-classification.md)에 차단·전달 범위를 정리했다. 차단 옵션이나 Anthropic 전용 서비스는 지원 기능으로 간주하지 않는다.

## 확인된 기능과 남는 한계

Verified: 실제 비대화형 JSON·stream-json, Read/Edit, Bash·PowerShell, stdio MCP, PNG 입력, WebFetch·WebSearch, Agent 선택, inline Workflow·StructuredOutput·부모 복귀, background 도구 결과 회수·TaskStop 뒤 worker 종료, 새 프로세스 resume, 실패 후 도구 미중복·이력 보존·정리를 확인했다. 모델 네 종류의 low 요청도 실제 성공을 확인했다.

Not verified / 제한:

- astra 최초 공개 단문 검사에서 전달 전 upstream error 1건이 있었고 이후 단독 검사는 성공했다. 최초 오류의 원인은 미확정이며 외부 서버 무장애를 보장하지 않는다.
- 기본 400K 창·320K 자동 압축 목표는 설정 계약이다. 기본값에서의 실제 발동·압축 후 전체 이력 보존은 미검증이며, 축소 창에서의 기존 실측과 구분한다.
- 실제 계정 회전, 모든 사용자 hook/plugin·permission/plan UI 조합, JPEG/GIF/WebP 실제 왕복, 프롬프트 캐시 실제 적중은 전수 검증하지 않았다.
- 취소·등록 교체·형제 격리의 합성 검사는 통과했지만 UI 취소 시점까지 연결한 전체 실측은 조건부다. 동적 symlink/junction 검사는 정책 차단으로 실행하지 않았다.
- 단일 completed 알림의 검증된 자동 복귀는 지원하지만 다중·실패·취소 알림 자동 복귀는 지원하지 않는다. Workflow는 inline 신규 실행과 이미 완료된 캐시 재사용의 증거가 있으며, 캐시 미적중 resume·중첩·custom agentType은 보장하지 않는다.
- Anthropic 서버 전용 기능, 비스트리밍 API, 서버 실행 도구·첨부/PDF·미지원 context edit·sampling 필드는 지원하지 않는다. 신규 native 버전과 Codex 비공개 backend 변경은 재검증이 필요하다. `CLI_VERSION_UNVERIFIED` 안내를 숨기지 않는다.

## 검증과 무결성

```powershell
pwsh -NoProfile -NonInteractive -File src/run-node-tests.ps1 -Root . -TestFiles 'src/test-*.mjs'
pwsh -NoProfile -NonInteractive -File verification/verify-native-headless.ps1 -Live -Case text
pwsh -NoProfile -NonInteractive -File verification/verify-native-headless.ps1 -Live -Case workflow
```

첫 명령은 검토된 로컬 회귀이며 실제 backend를 호출하지 않는다. 기본 러너의 `review-diff`는 PATH 부재로 notRun을 표시한다. 별도 검토된 Git 경로의 제한 환경에서는 실제 검사가 통과했다. native Workflow와 symlink의 합성 suite notRun을 실검사 성공으로 세지 않는다.

`-Live` 검사는 기존 로그인으로 공개 고정 fixture를 실제 전송하며 사용량이 발생한다. 새 작업 전용 프로필을 사용하고 프로세스당 최대 120초로 제한한다. 사용자 대화·프로필을 복사하지 않는다. `failure-resume`은 전달 후 오류를 의도적으로 1회 주입해 보존·복구를 확인하는 별도 사례다.

ZIP 옆 `manifest.json`은 소스 commit, 파일별 크기·SHA256, ZIP SHA256을 기록한다. SHA256은 손상 검사용이며 서명을 대신하지 않는다. 코드·검사·이 안내만 묶고 사용자 상태·프로필·Git 이력·감사 기록·과거 프롬프트·생성된 schema는 포함하지 않는다. 원본 Git checkout에서는 `pwsh -NoProfile -NonInteractive -File verification/build-release.ps1`로 같은 commit의 ZIP을 다시 만들 수 있다. 압축을 푼 배포본에는 Git 이력이 없어 빌더를 실행하지 않는다.
