# v0.5.2 — native auto 분류기와 세션 정책

**출하·실제 설치 확인 완료(2026-09-27).**
태그 `v0.5.2`는 `921e254de05932ae656f9d04077aa42be3f91695`이며
[GitHub Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.5.2)는 latest·정식 릴리스다.
[PR #157](https://github.com/wotjr1649/Clauduct/pull/157).
Claude Code 2.1.283, Codex CLI 0.157.1, Go 1.27.1 Windows 기준이다.

## 바뀐 것

auto 모드에서 분류가 필요한 행동이 `Classifier unavailable`로 끝나던 경로를 구현했다.
native가 만드는 GPT block 요청을 별도로 확인해 `gpt-6-luna/high`로 보내고 판정 응답을 native에 돌려준다.
일반 Sonnet→Terra/high, Haiku→Luna/max 선택은 그대로다. 모델·effort를 알 수 없거나 관측한
classifier 형식과 다르면 backend 전송 전에 거부한다.

native 내장 규칙을 `$defaults`로 유지하면서 두 hard_deny 조건을 자식 `--settings`에 추가한다.
정확한 사용자 승인 없는 원격 변경, 공개 내용·대상·후속 효과·설치 범위·프로세스 소유가 미확인인
고영향 작업을 차단한다. 사용자 규칙도 보존한다. native 실행 파일과 전역/managed 설정은 바꾸지 않는다.
정확한 코드와 관문은 [호환성 문서](COMPATIBILITY.md#v052--auto-모드-분류기149-재검증출하)에 있다.

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

TUI의 주 모델 도구 제안은 합성 fixture이고 분류 판정만 실제 backend다. 일반 모델 작업 전체의
인수를 뜻하지 않으며 아래 순수 태그 출하 검사와도 별도다. 독립 표본의 정상 프로세스 종료·로컬 설치 오차단 두 건은
남아 있으며, 표본 합격을 보편적 안전 보장으로 표현하지 않는다. 초기 잘못된 severity 캡처와
실패 결과도 보존했다. B 사용량은 이전 109회와 아래 출하 세션 5회를 포함해 959/1,000회다.

## 출하 검사와 되돌림

[PACKAGING.md](PACKAGING.md#41-릴리스-만들기)의 순서로 다음을 확인했다.

| 검사 | 결과 |
|---|---|
| 재현 빌드 | 깨끗한 태그를 독립 캐시로 두 번 빌드해 바이트 동일. Go 1.27.1·CGO=0·trimpath, `v0.5.2`/`921e254`, `+dirty` 없음 |
| 바이너리 | `clauduct.exe` SHA-256 `4b94e69f799dadd74b3436d7200c3a4db85d200c8cd10f348ac3a70d22ebd17f` |
| 격리 설치 | 새 설치·v0.5.1 위 교체·v0.5.1 되돌림, 파일 집합·digest·신원·사용자 PATH 불변 |
| 순수 자산의 실제 backend | 입력·`/clear`·background 자식 위임 5회. 부모 luna/max·자식 luna/low, API 실패 0, hook 설치·정상 종료·자원 해제 |
| 공개 자산 | latest·정식·자산 4개, API digest와 내려받은 바이트·SHA256SUMS·신원 일치 |
| updater | v0.5.1 → v0.5.2, 다음 native 실행의 `.old` 정리, 두 번째 업데이트 무변경. v0.3.5 updater의 누락 자산 거부·설치 보존 후 새 installer의 정상 전환 |
| 실제 설치본 | 기본 설치 경로 `%USERPROFILE%\.local\bin\clauduct.exe`를 v0.5.1에서 업데이트. 위 SHA-256·태그·commit 일치, native 2.1.283 실행, 두 번째 업데이트 무변경·`.old` 없음·사용자 PATH 불변 |

되돌림은 검증한 빌드 전체를 v0.5.1로 교체하고 새 세션을 시작하는 방식이다.
v0.5.1에서는 auto 판정이 필요한 행동이 다시 거부된다. 새 classifier 경로를 유지하면서
추가 규칙만 제거하는 조합은 검증한 후보가 아니다. 전역 파일은 바꾸지 않아 별도 복구가 필요 없다.
