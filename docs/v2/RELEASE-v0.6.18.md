# v0.6.18 — Claude Code 2.1.332 auto 모드 호환 수정

Claude Code 2.1.332로 업데이트한 뒤 auto 모드에서 분류가 필요한 도구 실행이 차단될 수 있던 문제를 고쳤다. [#414](https://github.com/wotjr1649/Clauduct/issues/414), [PR #415](https://github.com/wotjr1649/Clauduct/pull/415).

## 바뀐 것

- **native 2.1.331과 2.1.332의 분류 요청을 모두 받는다.** 2.1.332가 transcript 앞에 붙이는 분류기 표식 한 개를 받아들이고 2.1.331의 기존 요청 형식도 지원한다. 설정된 분류기 모델과 effort는 두 형식에 동일하게 적용된다. [호환성 문서](COMPATIBILITY.md#v0618--claude-code-21332의-auto-모드-분류기).
- **영향받는 조합과 업데이트 방법.** v0.6.17 이하에서 Claude Code 2.1.332를 쓰면 auto 모드의 분류 요청이 `AUTO_MODE_CLASSIFIER_UNVERIFIED`(HTTP 400)로 거부되어 분류 판정이 필요한 도구 실행이 차단된다. 거부는 backend 전송과 과금 예약 전에 일어난다. 분류 요청은 auto 모드에서만 나오므로 다른 권한 모드와, auto 모드에서 native가 분류 없이 허용하는 행동은 영향이 없다. native 2.1.331은 v0.6.17에서도 영향이 없다. `clauduct --update`(무인 실행은 `clauduct --update --yes`)로 v0.6.18을 받으면 해결된다. 업데이트 전에는 v0.6.17이 검증한 native 2.1.331을 유지한다. [업데이트 절차](PACKAGING.md#51-업데이트).

## 출하 검사

태그·commit, 후보 `clauduct.exe`와 `clauduct-probe.exe`, native 버전과 SHA-256에 결속한 `GATE-T1` 결과는 발행 뒤 이 문서에 기록한다. v0.6.17의 `GATE-T1`은 native 2.1.331에서 실행했고 이번에는 설치된 2.1.332에서 실행한다. native 이미지가 달라 `c1` 도구 표면 재측정 결과도 함께 기록한다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | 발행 후 기록 |
| `clauduct.exe` | 발행 후 기록 |
| `clauduct-probe.exe` | 발행 후 기록 |
| `install.ps1` | 발행 후 기록 |
| `uninstall.ps1` | 발행 후 기록 |
| `SHA256SUMS` | 발행 후 기록 |

## 알려진 제한

- auto 모드 분류 요청은 transcript 구조와 단계별 응답 조건을 검증한 뒤 전달한다. 측정한 형식 밖의 선행 블록은 `AUTO_MODE_CLASSIFIER_UNVERIFIED`로 거부한다. [분류 요청 형식](COMPATIBILITY.md#v0618--claude-code-21332의-auto-모드-분류기).
- **파일 기반 Workflow(`named`·`scriptPath`)는 native 2.1.332에서도 거부한다.** 검증한 이미지는 여전히 2.1.325뿐이다. 2.1.332 측정(2026-11-25)에서도 Workflow 내부 Read가 파일별 Read 금지와 전역 Read 금지를 지키지 않았다. 이 이미지의 파일 소스는 `CLAUDUCT_WORKFLOW_SOURCE_IMAGE_UNVERIFIED`로 Read 전에 거부한다. inline `script`는 계속 지원한다. [#403](https://github.com/wotjr1649/Clauduct/issues/403), [호환성 문서](COMPATIBILITY.md).
- 공통 제한은 [알려진 제한 원장](COMPATIBILITY.md#알려진-제한-원장-v0618)에 있다.
