# v0.3.2 출하 검증

2026-09-23. 태그 `v0.3.2`는 `5c2e719`(#72 병합 커밋)을 가리킨다. Go 1.27.1, `CGO_ENABLED=0`,
`-trimpath`, native Claude Code 2.1.280이다. 발행한 Release는
[v0.3.2](https://github.com/wotjr1649/Clauduct/releases/tag/v0.3.2)다.

## 빌드

`97a03bb`(#71 병합)에 태그를 달고 빌드했다. 두 빌드가 바이트 단위로 같았지만 `clauduct-dev version`은
`0.3.1`이었다. 버전은 VCS 태그가 아니라 `buildinfo.Version` 상수에서 오는데, #71에 상수 변경이
빠져 있었다. 발행 전에 멈추고 로컬 태그를 지웠다. 상수만 바꾼 #72를 CI 통과 뒤 병합하고
`5c2e719`에 다시 태그를 달았다.

태그의 깨끗한 checkout 두 곳에서 빌드했고, 두 번째는 별도 build cache를 썼다. 세 바이너리는 바이트
단위로 같았다. `clauduct-dev version`은 `0.3.2`, commit `5c2e719b8af0d1294ac6e19a4d2909a7c5e7b7a8`였고
`+dirty`는 없었다. `install.ps1`과 `uninstall.ps1`은 v0.3.1과 같은 bytes다.

## 발행 전

- [격리 설치](ship-install.ps1): 새 설치, v0.3.1 발행 자산 설치, v0.3.2로 업데이트, v0.3.1로 되돌리기.
  모든 단계에서 digest와 버전 스탬프가 맞았고 `.old` 파일과 PATH 변화는 없었다
  ([결과](ship-install.txt)).
- [실제 backend 세션](ship-session.mjs): 설치한 v0.3.2의 `clauduct.exe`로 입력, `/clear`, 백그라운드
  자식 위임을 stream-json으로 보냈다. `gpt-5.6-luna`/`low`, `CLAUDE_CODE_MAX_RETRIES=0`, 작업·profile·TEMP는
  공개 폴더다. backend 시도 5회, 거부·끊김·실패 0, `/clear` 뒤 세션 변경, 자식 보고 도착, native exit 0,
  hook 설치, 클라이언트 2.1.280 `verified=true`였다([결과](ship-session.txt)).
- `main` CI(`5c2e719`): Go 일반·race 각 19 package와 오프라인 증거 검사, httpguard Linux·macOS,
  Node 130 통과·1 skip.

## 발행 후

Release를 만든 직후 태그 기준 API(`releases/tags/v0.3.2`)는 4분 넘게 자산 0개를 보였다. Release ID와
`releases/latest`의 API는 자산 6개(`uploaded`)를 보였고, 다운로드 URL도 모두 200이었다. 그래서 자산은
다운로드 URL로 받았다.

- 받은 자산 6개는 빌드한 파일과 바이트 단위로 같았다. Release API의 digest는 `SHA256SUMS`와 같았다.
- [업데이트 검사](ship-update.ps1)([결과](ship-update.txt)):
  - `install.ps1 -Tag v0.3.2`가 GitHub에서 설치했다.
  - 격리된 v0.3.1에서 `clauduct --update --yes`를 실행해 v0.3.2로 교체했다.
  - 이전 프로세스가 지우지 못한 `.old` 하나는 다음 실행에서 사라졌다.
  - 두 번째 `--update`는 `already current`였다. PATH 변화는 없었다.

사용자의 실제 설치본(`~/.local/bin`)은 바꾸지 않았다.
