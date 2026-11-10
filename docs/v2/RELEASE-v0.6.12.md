# v0.6.12 — `sonnet`을 GPT-6.1 Terra로

2026-11-10. 범위는 [#371](https://github.com/wotjr1649/Clauduct/issues/371)이다. 남은 제한과 다시 볼 조건은
[COMPATIBILITY.md](COMPATIBILITY.md)의 3절 "알려진 제한 원장"이 관리한다.

## 바뀐 것

- **`sonnet` 별칭과 `terra` 키가 `gpt-6.1-terra`를 가리킨다**([#371](https://github.com/wotjr1649/Clauduct/issues/371))
  - `claude-sonnet-` 계열과 `clauduct-terra` 위임 메뉴도 같다. 공장 `modelDefaults`의 `gpt-6.1-terra` effort는 medium이다.
  - `gpt-5.6-terra`는 은퇴시키지 않는다. 전체 ID, `terra56` 키, `clauduct-terra56` 메뉴로 계속 쓴다(Agent 인자에는 `sonnet` 단계).
  - **이미 있는 `~/.clauduct/settings.json`은 바뀌지 않는다.** 설정 동기화는 빠진 최상위 키만 채우므로, 기존 파일의
    `modelMapping.sonnet`이 `gpt-5.6-terra`이면 그대로 쓴다. 기존 파일의 `modelDefaults`에는 `gpt-6.1-terra` 항목이 없어서,
    `clauduct-terra` 메뉴나 직접 고친 매핑으로 `gpt-6.1-terra`를 쓰면 계정 기본 effort(2026-11-10 기준 low)로 실행된다. 공장값과
    같게 쓰려면 매핑과 `modelDefaults` 항목을 함께 고친다([SETTINGS](SETTINGS.md)).
  - 알 수 없는 모델·effort를 거부하고 기본값으로 대체하지 않는 계약은 그대로다.
- **분류기 공장값은 `gpt-5.6-terra/low`로 둔다.** `gpt-6.1-terra`를 low·medium·high로 같은 공개 핵심 표본(74개, 위험 32)에
  측정했으나 통과한 pair가 없다. 측정 결과는 시작 안내 표에 넣었다. 그 pair를 `classifier_model`로 고르면 시작할 때 한 줄로
  알리고 설정대로 쓴다.

  | pair | 결과 |
  |---|---|
  | `gpt-6.1-terra/low` | 위험 허용 0, 판정 거부 0. 핵심 정상 표본 하나(승인된 태그 push)를 차단해 불합격 |
  | `gpt-6.1-terra/medium` | 13번째 요청의 보안 표본을 backend가 `cyber_policy`로 거부해 판정 미완료. 완료한 12개에서 위험 허용 0 |
  | `gpt-6.1-terra/high` | medium과 같음 |

## 출하 검사

전체 관문(A11)과 실제 backend 18묶음(Phase C)은 다시 실행하지 않았다. 소유자가 #371에서 승인한 예외다(2026-11-10). 대신
바뀐 동작을 아래처럼 확인했다.

- **등록 측정**(`clauduct --dev --probe accept gpt-6.1-terra --send`, 실제 backend 8회): low·medium·high·xhigh·max 수락.
  `ultra`는 이 빌드가 보내지 않는 effort라 요청 전에 거부했다(backend 수락 여부는 재지 않음). 도구 호출·결과 왕복과 이미지
  입력 정상, 로컬 토큰 계수 5/5 일치(`countValidated`). 짧은 프롬프트에서 reasoning이 나오지 않아 이 측정의 reasoning 왕복은
  NOT_RUN이다.
- **`sonnet` 경로**(후보 빌드, 설정 파일이 없는 격리 홈의 공장값, 실제 backend): `--model sonnet` print 2회의 생성 요청 3개가
  모두 `gpt-6.1-terra/medium`이었고 거부 0. 도구 호출이 있는 실행에서 첫 응답의 reasoning 기록이 다음 요청에 다시 실렸고
  backend가 받았다. native가 stderr에 `[claude-code:unrecognized_model]` 한 줄을 쓰지만 v0.6.11의 `gpt-5.6-terra`·
  `gpt-6.1-sol`에서도 같다(응답·라우팅 영향 없음).
- **분류기 측정**: 위 표. 실제 backend 138회.
- 단위·race 검사, 문서 인용 검사: 결과는 발행 기록에서 채운다(PR CI 포함).
- 출하 빌드·발행 검사([PACKAGING](PACKAGING.md#41-릴리스-만들기)): 발행 기록에서 채운다.

## 알려진 제한

- v0.6.11의 알려진 제한은 그대로다([RELEASE-v0.6.11](RELEASE-v0.6.11.md)). 이 릴리스는 그 항목을 해소하지 않는다.
- 이 릴리스는 전체 관문을 거치지 않았다. 위 출하 검사는 바뀐 모델 표의 동작만 확인한다.
