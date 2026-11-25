# v0.6.18 — Claude Code 2.1.332 auto 모드 호환 수정

Claude Code 2.1.332로 업데이트한 뒤 auto 모드에서 분류가 필요한 도구 실행이 차단될 수 있던 문제를 고쳤다. [#414](https://github.com/wotjr1649/Clauduct/issues/414), [PR #415](https://github.com/wotjr1649/Clauduct/pull/415).

## 바뀐 것

- Claude Code 2.1.332가 transcript 앞에 붙이는 분류기 표식 한 개를 받아들인다. 2.1.331의 기존 요청 형식도 지원한다. 설정된 분류기 모델과 effort는 두 형식에 동일하게 적용된다. [호환성 문서](COMPATIBILITY.md#v0618--claude-code-21332의-auto-모드-분류기).

## 출하 검사

태그·commit, 후보 `clauduct.exe`와 `clauduct-probe.exe`, native 버전과 SHA-256에 결속한 `GATE-T1` 결과는 발행 뒤 이 문서에 기록한다.

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
- 공통 제한은 [알려진 제한 원장](COMPATIBILITY.md#알려진-제한-원장-v0618)에 있다.
