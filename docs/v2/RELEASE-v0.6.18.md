# v0.6.18 — Claude Code 2.1.332 auto 모드 호환 수정

출시일과 정식 Release 링크: 발행 후 기록 (Release URL 자리표시, 발행 뒤 이 줄을 출시 문장으로 바꾼다)

Claude Code 2.1.332로 업데이트한 뒤 auto 모드에서 분류가 필요한 도구 실행이 차단될 수 있던 문제를 고쳤다. [#414](https://github.com/wotjr1649/Clauduct/issues/414), [PR #415](https://github.com/wotjr1649/Clauduct/pull/415).

## 바뀐 것

- **native 2.1.331과 2.1.332의 분류 요청을 모두 받는다.** 2.1.332가 transcript 앞에 붙이는 분류기 표식 한 개를 받아들이고 2.1.331의 기존 요청 형식도 지원한다. 설정된 분류기 모델과 effort는 두 형식에 동일하게 적용된다. [호환성 문서](COMPATIBILITY.md#v0618--claude-code-21332의-auto-모드-분류기).
- **영향받는 조합과 업데이트 방법.** v0.6.17 이하에서 Claude Code 2.1.332를 쓰면 auto 모드의 분류 요청이 `AUTO_MODE_CLASSIFIER_UNVERIFIED`(HTTP 400)로 거부되어 분류 판정이 필요한 도구 실행이 차단된다. 거부는 backend 전송과 과금 예약 전에 일어난다. 분류 요청은 auto 모드에서만 나오므로 다른 권한 모드와, auto 모드에서 native가 분류 없이 허용하는 행동은 영향이 없다. native 2.1.331은 v0.6.17에서도 영향이 없다. `clauduct --update`(무인 실행은 `clauduct --update --yes`)로 v0.6.18을 받으면 해결된다. 업데이트 전에는 v0.6.17이 검증한 native 2.1.331을 유지한다. [업데이트 절차](PACKAGING.md#51-업데이트).

## 출하 검사

태그·commit, 후보 `clauduct.exe`와 `clauduct-probe.exe`, native 2.1.332의 버전과 SHA-256(`e6652f9170a07df45f1db3e0520f1df0f7b385181ff74f70f49398a67b5fd320`)에 결속한 `GATE-T1` 12단계를 모두 통과했다. v0.6.17의 `GATE-T1`은 native 2.1.331에서 실행했고 이번에는 설치된 2.1.332에서 실행했다.

- `vet`, 전체 오프라인 Go 테스트, 태그 fixture `fixtures`·`fixtures-bg`, `structural`, native smoke 15행, `./internal/gateway` race를 통과했다.
- 실제 backend 분류기 표본 4종(공장 기본과 명시 override 각각의 허용·거부)을 통과했다. backend 예약은 총 14건이었다.
- native 이미지가 2.1.331에서 2.1.332로 바뀌어 `c1` 무과금 도구 표면 재측정을 실행했다. 도구 이름 48개와 print·TUI core 24개·27개가 이전 이미지와 같았고 이번 측정에서 T2를 요구하는 도구 차이는 없었다.

### 출하 빌드·발행 검사

- `build-release`의 독립 캐시 빌드 두 번이 바이트 단위로 일치했다.
- 발행 전 `ship-install`에서 새 설치, 발행된 v0.6.17에서 교체, v0.6.17로 되돌리기, 설정 파일과 사용자 PATH 보존을 확인했다.
- 발행 전 `probe limit --send` 3회에서 backend가 `max_output_tokens` 상한 16·48토큰을 지켰다(`HONOURED`). 예약 3건이 모두 성공했고 거부는 없었다.
- 발행 뒤 `verify-release`, `ship-update`, `ship-legacy`, `post-publish` 결과: 발행 후 기록.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | `v0.6.18` → `3f2000fb69a35f3f5e3b01db1caa8df5a10cb3ba` |
| `clauduct.exe` | `570b95891154d5065d5bdfc90f840b64d329d09a47a0f998e5d79b1038ab5fb5` |
| `clauduct-probe.exe` | `92ea033bc313afcc5056d8b73c63ac075e36b29b0ea7fb509bbe40e4224cc5ce` |
| `install.ps1` | `8911dd3c6350d02a4490250734fd7362edb7fc1e40c87891916ecbc0633c6f55` |
| `uninstall.ps1` | `a88d8f56719ee7ca7a199fd30214d8b74171a08c34c2d952cd60990b4d1ba295` |
| `SHA256SUMS` | `669c90a809840e0197946d88e1c80bc365060ea068f75a9a050b8b903b758619` |

Go 1.27.4, `CGO_ENABLED=0`, `-trimpath`. native를 쓴 출하 검사는 Claude Code 2.1.332에서 실행했다.

## 알려진 제한

- auto 모드 분류 요청은 transcript 구조와 단계별 응답 조건을 검증한 뒤 전달한다. 측정한 형식 밖의 선행 블록은 `AUTO_MODE_CLASSIFIER_UNVERIFIED`로 거부한다. [분류 요청 형식](COMPATIBILITY.md#v0618--claude-code-21332의-auto-모드-분류기).
- **파일 기반 Workflow(`named`·`scriptPath`)는 native 2.1.332에서도 거부한다.** 검증한 이미지는 여전히 2.1.325뿐이다. 2.1.332 측정(2026-11-25)에서도 Workflow 내부 Read가 파일별 Read 금지와 전역 Read 금지를 지키지 않았다. 이 이미지의 파일 소스는 `CLAUDUCT_WORKFLOW_SOURCE_IMAGE_UNVERIFIED`로 Read 전에 거부한다. inline `script`는 계속 지원한다. [#403](https://github.com/wotjr1649/Clauduct/issues/403), [호환성 문서](COMPATIBILITY.md).
- 공통 제한은 [알려진 제한 원장](COMPATIBILITY.md#알려진-제한-원장-v0618)에 있다.
