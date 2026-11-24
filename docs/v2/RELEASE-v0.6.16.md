# v0.6.16 — native 인자 판정을 버전과 무관하게 단순화

2026-11-24 출시(태그 `v0.6.16` → `a450202423e69a8f0f544e90c4d61a13d44d98c4`, [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.16)). 범위는 native 인자 판정의 버전 고정 제거와 PDF 페이지 Read 안내다. native는 주마다 여러 번 갱신되므로 특정 실행 파일에 묶은 판정을 유지하지 않는다.

## 바뀐 것

- **모든 native에서 같은 인자 판정을 쓴다.** argv 어디든 정확한 `--bg`·`--background` 토큰이 있으면 background 요청으로 판정한다. 옵션 값 자리(`--name --bg`)나 `--` 뒤의 `--bg`도 background 요청이 된다. 이 토큰이 없으면 선두 `rm`·`stop`·`logs`만 관리 명령 후보다. 선행 옵션 뒤 관리 명령과 선두 `kill`은 세션 경로를 탄다. v0.6.15까지 native 2.1.330 실행 파일에만 적용하던 정밀 판정 표를 제거했다. [#331](https://github.com/wotjr1649/Clauduct/issues/331).
- **긴 PDF 페이지 Read에 필요한 Poppler 설치 방법을 안내한다.** [README](../../README.md), [COMPATIBILITY](COMPATIBILITY.md).

## 출하 검사

태그·commit, 후보 `clauduct.exe`와 `clauduct-probe.exe`, native 2.1.330의 버전과 SHA-256에 결속한 `GATE-T1`을 통과했다.

- `vet`, 전체 오프라인 Go 테스트, 태그 fixture `fixtures`·`fixtures-bg`, `structural`, native smoke 15행: PASS.
- 바뀐 동작의 실제 backend 표본(인자 판정): `--bg`·`--background`는 background 작업, 선두 `logs`·`stop`·`rm`은 모델 예약 없는 관리 명령으로 처리됐다. `--name --bg`·`-- --bg`는 알려진 제한대로 `BACKGROUND_DISPATCH_UNVERIFIED`였다. PASS.
- `c1` 무과금 도구 표면 재측정: 도구 이름·core가 기준선과 같다.
- race는 동시성 코드 변경이 없어 실행하지 않았다(NOT_RUN).

### 출하 빌드·발행 검사

- `build-release`: 두 독립 빌드의 바이트가 같고 probe digest가 저장소 값과 일치한다.
- 발행 전 `ship-install`: 새 설치, v0.6.15에서 업데이트, 되돌리기, 사용자 PATH 불변을 확인했다.
- 발행 전 `probe limit --send`(3회): `max_output_tokens` 상한을 지킨다.
- 발행 뒤 `verify-release`, `ship-update`, `ship-legacy`(v0.3.5)를 모두 통과했다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | `v0.6.16` → `a450202423e69a8f0f544e90c4d61a13d44d98c4` |
| `clauduct.exe` | `3fa001482453a122c530afecc176e42b1f9d0c8558571fed0834491b66f4d0af` |
| `clauduct-probe.exe` | `4a7f023c0deebd8877e359e9991c326ab5fc59200f7195b55b0fca7b8eb4b5be` |
| `install.ps1` | `6720976f89e9016a33bd046f02bcf36e1acfdb411097b93c1d8a443f55356c58` |
| `uninstall.ps1` | `a88d8f56719ee7ca7a199fd30214d8b74171a08c34c2d952cd60990b4d1ba295` |
| `SHA256SUMS` | `214e3c5f43d021a13fcc4a88ea63f8e9697620e78b1a6bcba40955084bffa2ca` |

Go 1.27.4, `CGO_ENABLED=0`, `-trimpath`. native를 쓴 검사는 모두 Claude Code 2.1.330에서 돌았다.

## 알려진 제한

- **옵션 값이나 `--` 뒤에 `--bg`를 쓰면 background 요청으로 판정한다.** native는 `--name --bg`·`-- --bg`를 foreground로 처리하므로 두 판단이 갈려 `BACKGROUND_DISPATCH_UNVERIFIED`로 실패한다. 이름이나 프롬프트에 `--bg`라는 문자열을 단독 인자로 쓰지 않으면 영향이 없다.
- **10쪽이 넘는 PDF를 페이지 지정으로 읽으려면 Poppler(`pdftoppm`)가 필요하다.** native Claude Code와 같은 동작이다.
- background의 native telemetry·계정 조회와 Clauduct job 종료 시 복원한 resident가 함께 끝날 수 있는 제한은 계속 적용된다. [v0.6.14 알려진 제한](RELEASE-v0.6.14.md#알려진-제한).
