# Windows 설치와 업데이트

설치기는 현재 사용자 홈의 `.local\bin`에 `clauduct.cmd`를 만들고, 제품 파일은 `clauduct\versions\<전체 커밋 SHA>`에 설치한다. Windows에서 `clauduct`와 `Clauduct`는 같은 명령이다. 호출한 작업 디렉터리를 유지하므로 프로젝트로 이동한 뒤 실행한다.

## 필요한 기존 프로그램

- Windows와 Node.js 24 이상. `node.exe`가 PATH에 있어야 한다.
- native Claude Code: 사용자 홈 `.local\bin\claude.exe`를 우선하고 절대 PATH 항목의 `claude.exe`를 탐색한다.
- Codex CLI: 사용자 홈 `AppData\Local\Programs\OpenAI\Codex\bin\codex.exe`, PATH의 `codex.exe`, npm의 `@openai/codex` 순으로 탐색한다. npm은 알려진 prefix의 package 이름과 `bin/codex.js` 항목을 확인하고 Node로 직접 실행한다. 상대 PATH와 현재 작업 디렉터리 항목은 CLI 탐색에서 제외한다.
- 실사용에는 기존 Codex 로그인과 OS 사용자 홈 `.codex`의 파일 credential store가 필요하다. 설치기는 로그인 여부를 판정하거나 인증을 복제하지 않는다.

설치 전 CLI의 `--version`과 제품의 `--dry-run`을 실행한다. 빠진 프로그램은 오류로 안내하며 자동 설치하지 않는다. native 전역/프로젝트 설정은 기존 Claude Code가 읽는다. 설치기는 `settings.json`, Codex 설정, 인증 파일을 변경하지 않는다.

## 로컬 릴리즈 설치

릴리즈 폴더에는 `install.ps1`, `Clauduct-windows-x64.zip`, `manifest.json`, `SHA256SUMS`가 함께 있다. 해당 폴더의 PowerShell에서 실행한다.

```powershell
.\install.ps1 -PackagePath .\Clauduct-windows-x64.zip -ManifestPath .\manifest.json
```

기본 설치는 사용자 PATH에 `.local\bin`을 중복 없이 추가한다. 관리자 권한이나 시스템 PATH 변경은 필요 없다. 설치 후 새 터미널을 열어 `clauduct --dry-run`으로 확인한다. 기존 터미널의 부모 프로세스 PATH는 자식 설치 프로세스가 변경할 수 없다. 동명 alias/function 또는 앞선 PATH 프로그램이 있으면 `Get-Command clauduct -All`로 확인하고 설치된 `clauduct.cmd`의 전체 경로로 실행한다.

별도 폴더에서 시험하고 사용자 PATH를 유지하려면 다음처럼 지정한다.

```powershell
.\install.ps1 -PackagePath .\Clauduct-windows-x64.zip -ManifestPath .\manifest.json -InstallRoot 'D:\Tools\Clauduct-bin' -NoPathUpdate
& 'D:\Tools\Clauduct-bin\clauduct.cmd' --dry-run
```

설치된 명령은 설치 당시 Node의 절대 경로를 사용한다. Node를 다른 위치로 옮겼다면 설치기를 다시 실행한다. 기존 버전과 상태 기록은 자동 삭제하지 않는다. 같은 버전 재설치는 파일 해시를 확인한다. 수정된 제품 파일, 알 수 없는 기존 설치 폴더, 동명 실행 파일, 남아 있는 설치 잠금은 덮어쓰지 않는다. 잠금 오류는 다른 설치 프로세스 종료와 해당 설치 상태를 먼저 확인해야 한다. 전원 손실 중 자동 복구까지 보증하지 않는다. 같은 저장 세션을 두 버전에서 동시에 실행하지 않는다.

## GitHub Release 설치

> **`latest`로는 더 이상 받을 수 없다 (2026-09-17 확인).** 이 문서의 Node 설치기는 `v0.1.0`
> 자산이고, `latest`는 이제 Go 바이너리 3개와 `SHA256SUMS`만 싣는 `v0.2.0`이다. 아래 원라이너를
> 그대로 실행하면 `install.ps1`을 찾지 못해 실패한다. **태그를 `v0.1.0`으로 고정해야 한다.**
> Go 빌드를 설치하려면 [docs/v2/PACKAGING.md](v2/PACKAGING.md) 5장을 본다 — 설치는 파일 복사다.

다음 주소는 Release에 네 파일을 게시하면 사용할 수 있는 배포 경로다. Release가 없는 경우 다운로드는 실패한다. 게시된 버전과 검증 범위는 해당 Release 설명을 확인한다.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm 'https://github.com/wotjr1649/Clauduct/releases/download/v0.1.0/install.ps1' | iex"
```

조직 실행 정책은 이 명령보다 우선할 수 있다. 설치기 자체는 실행 정책을 변경하지 않는다. `irm | iex`는 받은 설치 스크립트를 실행하므로 최초 스크립트의 신뢰는 배포 주소와 게시자에 달려 있다. ZIP 및 개별 파일의 SHA256 검증은 독립적인 서명을 대신하지 않는다. 다운로드는 고정 GitHub 대상과 검토한 asset redirect 호스트에만 허용하고 크기·시간·redirect 횟수를 제한한다. latest는 한 번 확정한 release tag에서 ZIP과 manifest를 받는다. 고정 버전은 다운로드한 설치기에 `-Version <tag>`를 지정한다.

## 검증 범위와 제한

PowerShell 7에서 공개 합성 패키지로 신규 설치, 업데이트, 이전 버전 보존, 실패 시 명령 보존, 잠금/충돌/변조 거부, 다른 작업 디렉터리·대소문자·공백 인수를 검사한다. PATH 문자열 계산은 합성 값으로 검사하며 실제 사용자 PATH 쓰기는 개발 검증에서 제외한다. 실제 제품 패키지의 로컬 설치와 `--dry-run` 증거는 릴리즈 작업 기록에 별도로 연결한다. 모델 호출 성공과 설치 성공은 구분한다.

설치기는 PowerShell 5.1 문법을 목표로 작성했지만 이 머신의 Windows PowerShell 5.1 검사는 실행 정책의 `UnauthorizedAccess`로 시작 전에 차단됐다. 정책을 우회하지 않았으므로 5.1 실행 호환성은 미검증이다. 다른 머신과 실제 사용자 PATH 갱신도 검증 완료로 표시하지 않는다. 온라인 다운로드 확인은 각 Release의 게시 후 검증 기록과 구분한다. 빌드 스크립트는 PowerShell 7이 필요하다.

설치 경로의 reparse point와 `%`, `!`, 따옴표, 줄바꿈을 거부한다. 설치 ZIP의 경로 탈출, 예약 장치명, 중복 경로, 비일반 파일 유형과 과대 파일을 거부한다. 인증·개인 profile·과거 세션은 릴리즈에 포함하지 않는다.
