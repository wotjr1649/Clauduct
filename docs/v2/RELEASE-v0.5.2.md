# v0.5.2 후보 — native auto 분류기와 세션 정책

**미출하(2026-09-27).** 아래는 검증한 로컬 후보이며 태그·GitHub Release·설치 교체는 아직 없다.
Claude Code 2.1.283, Codex CLI 0.157.1, Go 1.27.1 Windows 기준이다.

## 바뀐 것

auto 모드에서 분류가 필요한 행동이 `Classifier unavailable`로 끝나던 경로를 구현했다.
native가 만드는 GPT block 요청을 별도로 확인해 `gpt-6-luna/high`로 보내고 판정 응답을 native에 돌려준다.
일반 Sonnet→Terra/high, Haiku→Luna/max 선택은 그대로다. 모델·effort를 알 수 없거나 관측한
classifier 형식과 다르면 backend 전송 전에 거부한다.

native 내장 규칙을 `$defaults`로 유지하면서 두 hard_deny 조건을 자식 `--settings`에 추가한다.
정확한 사용자 승인 없는 원격 변경, 공개 내용·대상·후속 효과·설치 범위·프로세스 소유가 미확인인
고영향 작업을 차단한다. 사용자 규칙도 보존한다. native 실행 파일과 전역/managed 설정은 바꾸지 않는다.
정확한 코드와 관문은 [호환성 문서](COMPATIBILITY.md#v052--auto-모드-분류기149-재검증한-후보-미출하)에 있다.

backend가 지원하지 않는 `stop_sequences`는 로컬 응답 번역에서 처리한다. 정지 문자열과 이후
텍스트·도구 호출을 전달하지 않되 원본 출력 검증과 실제 usage는 유지한다. backend 생성 조기 종료나
과금 절감을 뜻하지 않는다. `thinking: disabled` 요청의 출력 한도는 reasoning을 제외하고 검사한다.

## 검증 결과

| 검사 | 관측 결과 |
|---|---|
| 기존 70개 | Luna/high: 위험 허용 0/30, 정상 허용 29/30, 핵심 정상 6/6, 형식 오류 0 |
| 후보 고정 후 독립 70개 | Luna/high: 위험 허용 0/30, 정상 허용 28/30, 핵심 정상 6/6, 형식 오류 0 |
| 민감 사례 반복 | 위험 4개 차단·핵심 정상 6개 허용. 더 빠른 Sol/low는 핵심 두 건을 차단해 제외 |
| 실제 native TUI + 실제 classifier backend | native가 요청한 model/effort를 유지한 채 제품에서 Luna/high 선택. 전체 gateway 응답을 native에 반환. 허용 marker 생성·거부 marker 보존과 정상 종료 확인 |
| 실제 후보의 기본 설정 | 사용자 설정 없이 두 규칙 적용. 내장 규칙과 사용자 추가 규칙 보존, backend 0회 |
| 로컬 회귀 | gofmt, vet(기본·policy_evidence·runtime_evidence), build, 전체 Go 테스트와 race PASS. 정책 두 파일은 전체 검사를 통과한 overlay와 적용 후 바이트 동일 |
| 수정 필요성 | stop/출력 한도/classifier 선택을 제거한 검사는 예상대로 실패 |

TUI의 주 모델 도구 제안은 합성 fixture이고 분류 판정만 실제 backend다. 일반 모델 작업 전체나
순수 태그 출하 검사를 대신하지 않는다. 독립 표본의 정상 프로세스 종료·로컬 설치 오차단 두 건은
남아 있으며, 표본 합격을 보편적 안전 보장으로 표현하지 않는다. 초기 잘못된 severity 캡처와
실패 결과도 보존했다. B 사용량은 이전 109회 포함 954/1,000회다.

## 출하 전 남은 것과 되돌림

순수 태그 재현 빌드, 격리 설치·되돌림, 해당 자산의 실제 backend 세션, 공개 Release와
다운로드 digest·updater 확인, 실제 설치본 교체·확인은 아직 실행하지 않았다.
공개·설치 효과의 승인을 받은 뒤 [PACKAGING.md](PACKAGING.md#41-릴리스-만들기)의 순서로 수행한다.

되돌림은 검증한 빌드 전체를 v0.5.1로 교체하고 새 세션을 시작하는 방식이다.
v0.5.1에서는 auto 판정이 필요한 행동이 다시 거부된다. 새 classifier 경로를 유지하면서
추가 규칙만 제거하는 조합은 검증한 후보가 아니다. 전역 파일은 바꾸지 않아 별도 복구가 필요 없다.
