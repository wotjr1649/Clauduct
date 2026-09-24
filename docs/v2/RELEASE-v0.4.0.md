# Clauduct v0.4.0

Windows x64용 Go V2 패키징 릴리스 후보다. 요청을 번역하고 보내는 방식은 v0.3.5와 같고, 바뀐 것은 설치되는 파일과
빌드가 자기 버전을 아는 방식이다. 배포 여부와 검사 결과는 해당 후보 commit의 출하 검증 기록으로 확인한다.

- **단일 바이너리.** 설치는 `clauduct.exe` 하나다(20.9 MB. v0.3.5의 세 파일은 합계 55.6 MB). 세션 hook, gateway의 PDF 렌더러,
  native가 찾는 `pdftoppm.exe`, 이 프로젝트 자신의 명령을 모두 이 파일이 맡는다([#112](https://github.com/wotjr1649/Clauduct/issues/112)).
  - 이 빌드 자신에 대한 명령은 첫 인자 `--dev` 뒤로 옮겼다: `clauduct --dev --version`, `--doctor`, `--usage`, `--probe`.
    `clauduct`가 첫 인자로 소유하는 것은 `--update`·`--usage`·`--uninstall`·`--dev`이고 나머지는 전부 native로 간다.
  - hook과 렌더러는 `--clauduct-hook`·`--clauduct-render-pdf`라는 예약 인자로 불린다. 사람이 칠 일은 없다.
- **전환(v0.4.x 동안).** 0.3.x의 `--update`는 세 이름이 모두 있는 릴리스만 받으므로, v0.4.x 릴리스는 `clauduct-hook.exe`와
  `clauduct-dev.exe`를 `clauduct.exe`의 바이트 동일 사본으로 함께 올린다. 사본은 이름이 말하는 역할을 하므로 0.3.x에서
  `clauduct --update`로 올라온 설치는 세 파일로 동작하고, `clauduct-dev version`도 그대로 된다. 새 `--update`와 `install.ps1`은
  `clauduct.exe`만 받고 교체가 끝난 뒤 두 사본을 지운다(지울 것을 확인 질문 전에 이름으로 보여준다). v0.5.0부터는 사본을 올리지
  않으며, 그때까지 0.3.x에 남은 설치는 `install.ps1`로 다시 설치한다. **설치·업데이트 전에 시작한 세션은 다시 시작한다** —
  사본이 지워지면 그 세션은 hook을 잃는다.
- **버전은 태그에서.** `go.mod`를 저장소 루트로 옮겨(module `github.com/wotjr1649/Clauduct`, 패키지 경로는 그대로) Go
  toolchain이 태그를 버전으로 찍는다. `clauduct --dev --version`은 `v0.4.0`처럼 태그 그대로 말하고(0.3.x는 `0.3.5`처럼 `v` 없이),
  태그 없는 commit은 pseudo-version, 수정된 트리는 `+dirty`다. 릴리스마다 버전 상수를 올리던 단계가 없어졌다
  ([#111](https://github.com/wotjr1649/Clauduct/issues/111)).
- **출하 검사.** 유지보수자 로컬의 출하 검사가 태그·commit을 인자로 받고, 설치 파일 목록을 릴리스 자신의 `install.ps1`과
  `SHA256SUMS`에서 읽는다. 릴리스마다 스크립트를 복사해 고치지 않는다([#122](https://github.com/wotjr1649/Clauduct/issues/122)).

기준 클라이언트 상수는 Claude Code 2.1.281, Codex CLI 0.153.4로 v0.3.5와 같다. 설치된 Codex가 다른 버전이면 요청은 그 버전을
싣고 나가며 막히지 않는다. 설치된 Codex를 따라 재측정하는 것은 v0.4.x 과제다([#121](https://github.com/wotjr1649/Clauduct/issues/121)).
모델 표와 위임 메뉴는 v0.3.5와 같다.

출하 구성은 Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`다. 자산은 `clauduct.exe`, 그 사본 `clauduct-hook.exe`·`clauduct-dev.exe`,
`install.ps1`·`uninstall.ps1`, `SHA256SUMS`다. 설치와 업데이트는 [패키징 안내](PACKAGING.md)를 따른다.

**되돌리기.** `install.ps1 -Tag v0.3.5`는 그 릴리스의 digest를 보고 세 프로그램을 모두 둔다(v0.3.5 자산의 `install.ps1`도 된다).
v0.4.0이 남기는 기록 중 v0.3.5가 읽지 못하는 것은 없다. 되돌리면 `--dev` 대신 `clauduct-dev`를 쓴다.

Windows 전용이며 서명하지 않는다. 지원 기능과 남은 조건은 [호환성 문서](COMPATIBILITY.md)를 참조한다.

## 출하 검사 (2026-09-24)

태그 `v0.4.0`은 `8f0ffd7`을 가리키고, [Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.4.0)에 자산 6개가 있다.
검증 기록은 공개 저장소에 두지 않으므로 결과만 적는다. 검사 스크립트는 이번부터 릴리스마다 복사하지 않고 태그와 commit만 넘겨 실행했다.

- 태그의 깨끗한 checkout 두 곳에서 빌드했고, 두 번째는 별도 build cache를 썼다. `clauduct.exe`는 바이트 단위로 같았다
  (`d168b811…`, 사본 둘도 같은 digest). `clauduct --dev --version`은 `v0.4.0`, commit `8f0ffd7643127ead3c348adaa5704c5c637f9db2`이고
  `+dirty`는 없다.
- 격리 설치로 새 설치(`clauduct.exe` 하나), v0.3.5 발행 자산 설치(셋), 새 설치 스크립트로 교체(하나, 옛 두 이름 제거), v0.3.5 스크립트로
  되돌리기(셋)를 차례로 했다. 모든 단계에서 파일 집합·digest·버전 스탬프가 맞았고 `.old` 파일과 PATH 변화는 없었다.
- 설치한 v0.4.0으로 실제 backend 세션을 돌렸다(시도 5회). 입력, `/clear`, 백그라운드 자식 위임(부모 `gpt-6-luna`/max, 자식
  `clauduct-luna`에 `effort: low`)을 보냈고 hook(`clauduct.exe --clauduct-hook`)이 자식을 등록했으며 거부·끊김·실패는 0이었다.
  발행 전에는 같은 코드의 개발 빌드로 SDK 위임 세션과 TUI를 10회 돌렸다. Codex CLI는 0.156.1이었다.
- 발행 후 받은 자산 6개는 빌드한 파일과 같았고, Release API의 digest는 `SHA256SUMS`와 같았다.
  `install.ps1 -Tag v0.4.0`으로 GitHub에서 설치했다. 격리된 v0.3.5의 `clauduct --update --yes`는 0.3.5 updater로 세 이름을 받아
  v0.4.0이 됐고(사본 `clauduct-dev.exe`가 `v0.4.0`을 말했다), 남은 `.old`는 다음 실행에서 사라졌다. 두 번째 `--update`는
  `already current`였다.
