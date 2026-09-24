# Clauduct v0.3.5

Windows x64용 Go V2 유지보수 릴리스 후보다. v0.3.3 재판정과 코드 읽기에서 나온 결함과 V1 대비 격차를 닫는다.
모델 표와 위임 메뉴는 v0.3.4와 같다. 배포 여부와 검사 결과는 해당 후보 commit의 출하 검증 기록으로 확인한다.

- **`-p`의 Ctrl+C.** 전에는 런처가 즉시 끝나 종료 보고가 없었고, 세션 plugin 디렉터리가 `%TEMP%`에 남았으며, 자식도 함께
  끝났다. 이제 `USER_CANCELLED`로 기록하고, 같은 이벤트를 받은 자식이 스스로 끝나기를 기다린 뒤 평소처럼 정리한다.
  종료 코드는 자식의 것이다. interactive 세션의 Ctrl+C는 그대로 native가 처리한다([#86](https://github.com/wotjr1649/Clauduct/issues/86)).
- **Codex 찾기.** `~\.local\bin`과 PATH 외에 Codex 앱의 설치 위치와 npm 설치가 싣는 native `codex.exe`도 찾는다.
  Node는 실행하지 않는다. 설치 스크립트의 사전 검증도 같다([#88](https://github.com/wotjr1649/Clauduct/issues/88)).
- **설치 스크립트 교체.** 세 파일을 한 세트로 바꾸고, 실패하면 원래 세트를 되돌린다(`INSTALL_SWAP_FAILED`).
  실행 중인 바이너리도 교체된다([#89](https://github.com/wotjr1649/Clauduct/issues/89)).
- **모르는 backend 출력 항목.** 전에는 응답에서 빠진 채 성공으로 끝났다. 이제 `UNSUPPORTED_OUTPUT`으로 거부하고,
  상태의 `events.outputItems`에 항목 종류를 남긴다([#85](https://github.com/wotjr1649/Clauduct/issues/85)).
- **V1 대비 격차**([#91](https://github.com/wotjr1649/Clauduct/issues/91)):
  - backend가 준 `Retry-After`를 세션 전체에 적용하고, 클라이언트에도 429와 함께 전달한다. 그 시각 전의 시도는 보내지 않고
    `UPSTREAM_RETRY_DEFERRED`로 거부하며 시도로 세지 않는다.
  - backend가 응답에 적은 model·effort가 보낸 route와 다르면 `MODEL_EFFORT_MISMATCH`로 거부한다. 표의 네 모델 모두
    보낸 이름 그대로 돌려주는 것을 먼저 확인했다.
  - `system`은 문자열 또는 text block만 받는다. 그 밖은 버리지 않고 거부한다.
  - WebSearch의 401은 저장된 토큰이 바뀐 경우에만 한 번 다시 시도한다.
  - cached 입력을 `cache_read_input_tokens`로 나눠 보고한다. 클라이언트의 `/cost`·`/context`에 보이는 입력 수가
    작아지고 cache read가 보이지만, 합계는 같다.
  - Workflow·SDK 대기 응답의 텍스트를 한 block으로 보낸다.
  - 잘못되었거나 32개를 넘는 web_search 도메인 필터는 잘라서 보내지 않고 거부한다(`UNSUPPORTED_TOOLS`).
  - 상태에 등록 만료·퇴출 수, 세션의 처음 실패, 정리 실패 여부를 추가한다.
  - 사용자가 멈춘 자식은 어느 경로로 재개해도 거부한다.
  - 동시 요청 admission의 memory budget은 v0.4.0 과제다. 첫 출력 전 keepalive를 보내지 않는 것은 의도다.
- **gateway 요청당 비용.** 먼저 측정했다. 요청당 약 15–25 ms로 backend 턴의 0.5–2%여서 바꾸지 않는다
  ([#110](https://github.com/wotjr1649/Clauduct/issues/110)).
- **유지보수 도구.** TUI 검사를 pseudo console에서 조작하는 `go/cmd/ptydrive`를 공개한다. 출하 자산은 아니다
  ([#75](https://github.com/wotjr1649/Clauduct/issues/75)).

기준 클라이언트 상수는 Claude Code 2.1.281로 v0.3.4와 같다.

출하 구성은 Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`다. `clauduct.exe`,
`clauduct-hook.exe`, `clauduct-dev.exe` 세 파일과 `SHA256SUMS`를 함께 사용한다.
설치와 업데이트는 [패키징 안내](PACKAGING.md)를 따른다.

**되돌리기.** v0.3.4로 바이너리를 되돌려도 v0.3.5가 남긴 기록 중 v0.3.4가 읽지 못하는 것은 없다. 상태 파일의 새 필드는
추가만 했다. 되돌리면 위 수정들이 함께 빠진다.

Windows 전용이며 서명하지 않는다. 지원 기능과 남은 조건은 [호환성 문서](COMPATIBILITY.md)를
참조한다.

## 출하 검사 (2026-09-24)

태그 `v0.3.5`는 `6bdbb82`를 가리키고, [Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.3.5)에 자산 6개가 있다.
검증 기록은 공개 저장소에 두지 않으므로 결과만 적는다.

- 태그의 깨끗한 checkout 두 곳에서 빌드했고, 두 번째는 별도 build cache를 썼다. 세 바이너리는 바이트 단위로 같았다.
  `clauduct-dev version`은 `0.3.5`, commit `6bdbb82dddb071ca36ccdb2431bed935c6665eed`이고 `+dirty`는 없다.
- 격리 설치로 새 설치, v0.3.4 발행 자산 설치, v0.3.5로 업데이트, v0.3.4로 되돌리기를 차례로 했다. 모든 단계에서
  digest와 버전 스탬프가 맞았고 `.old` 파일과 PATH 변화는 없었다.
- 설치한 v0.3.5로 실제 backend 세션을 돌렸다(시도 5회). 입력, `/clear`, 백그라운드 자식 위임(부모 `gpt-6-luna`/max,
  자식 `clauduct-luna`에 `effort: low`)을 보냈고 거부·끊김·실패는 0이었다. 발행 전에는 같은 트리의 개발 빌드로 20회를 돌렸다:
  네 모델의 `-p`, `-p` Ctrl+C, WebSearch, SDK 위임 세션, TUI.
- 발행 후 받은 자산 6개는 빌드한 파일과 같았고, Release API의 digest는 `SHA256SUMS`와 같았다.
  `install.ps1 -Tag v0.3.5`로 GitHub에서 설치했다. 격리된 v0.3.4의 `clauduct --update --yes`는 v0.3.5로 교체했고,
  남은 `.old`는 다음 실행에서 사라졌다. 두 번째 `--update`는 `already current`였다.
