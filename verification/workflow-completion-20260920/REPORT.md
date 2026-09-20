# S49 Workflow 복원·소스·기본 선택 검수

최신 수용 판정(2026-09-20): 사용자 S44와 같은 바이너리/native의 B OS 보충 TUI를 합쳐
합의한 지원 범위의 출시 적합은 합격이다. B 실제 실행·취소·회수, 재시작 후 A 재사용/B
재실행 0/C만 실행을 확인했다. [최종 수용 근거와 한계](S44-B-OS-ACCEPTANCE.md).
아래의 이전 수용 보류와 실패 기록은 당시 증거 범위를 보존한 이력이다.

## 판정 범위

동일 세션을 `claude --resume`으로 다시 열어 저장된 Workflow 결과를 재사용하는 동작은
현재 native 공식 문서에도 있다. `같은 세션`은 같은 OS 프로세스라는 뜻이 아니다.
다만 native 재개는 실패·중단한 작업을 다시 실행할 수 있다. Clauduct의 채택 정책은
시작한 작업을 자동 재실행하지 않으므로 의미가 다르다.
[native 공식 재개](https://code.claude.com/docs/en/workflows#resume-after-a-pause).

S49는 native `2.1.278`과 기존 Go 구현을 대상으로 한다. 사용자 최종 TUI 합격은 별도이며
프로세스 종료 0, 단위 검사 성공, `selection_verified`만으로 전체 합격을 선언하지 않는다.

## 구현

- native 프로세스 수거와 gateway drain 이후 Workflow 메타데이터를 저장한다. 기존 native
  script/journal/child metadata 해시, 모델·effort, native 종료 근거와 계획 prompt의 script 내
  위치만 남긴다. prompt, 결과 본문, 사용자 label을 별도 checkpoint에 복제하지 않는다.
- 재개 세션의 SessionStart가 확인한 프로젝트 경로에서만 복원한다. 변경된 script/journal/
  metadata, 다른 세션, 누락된 기록, 잘못된 offset·도구 정책은 거부한다. 과거 기록은
  `restored_evidence`이며 현재 작업 실행이나 결과 전달 완료로 표시하지 않는다.
- 독립 계획은 디스크의 배타적 claim을 기록한 후 미실행 단계만 재개한다. 별도 gateway의
  동시 재개와 launcher 재시작 후 중복 재개도 차단한다. claim 뒤 중단/거부가 발생하면
  불명확한 재개를 자동 반복하지 않는다.
- `scriptPath` 및 프로젝트/사용자 디렉터리의 named `.js`는 실제 native `Read`의 훅과
  권한 검사를 통과한 전체 텍스트만 사용한다. source 해시와 실제 native 저장 script를
  대조한 후 자식 라우팅을 허용한다. 파일 읽기 거부 시 직접 파일 읽기로 대체하지 않는다.
- custom `agentType`의 모델·effort 생략은 역할 정의와 실제 native turn을 대조한다.
  native metadata의 model은 호출 인자이므로 생략 시 빈 값일 수 있다. native가 확정한
  모델·effort를 별도로 확인하므로 요청값을 추측으로 대입하지 않는다.
- native `pipeline`과 중첩 `parallel`의 callback이 기존 `agent()` 선택·도구 제한을
  유지하는 것을 실제 native 프로세스로 검사한다. 별도 orchestration 엔진을 추가하지 않았다.
- status에 `workflowPersistence.saved/restored/failed`, `workflow_restore` 조건 관측을 추가한다.

## 실제로 남기는 한계와 대체 경로

| 범위 | 판단 |
|---|---|
| 강제 종료/전원 손실로 최종 checkpoint 자체가 없음 | 복원 성공으로 추측하지 않는다. 현재 구현은 수거 완료 뒤 저장된 근거를 요구한다. 정상 종료 후 같은 UUID 재개를 사용한다. 모든 종료 환경을 지원한다는 주장과 구분 |
| 이미 시작했으나 효과가 불명확한 작업의 재실행 | 사용자 정책상 자동 반복 금지. 외부 시스템의 멱등성·실행 결과 확인 없이 정확히 한 번 실행을 보장할 수 없음 |
| 임의 Workflow JS 재개 | 원본 JS를 재실행하지 않고 검증된 child report만 회수. 미실행 단계 실행은 `clauduct:plan-v1`의 독립 단계 계약 사용 |
| 자식 안에서 별도 Workflow 호출 | native workflow-subagent의 도구 제한에 Workflow가 포함됨. 이 제한을 제거하지 않음. 한 Workflow의 `pipeline`/중첩 `parallel`, 또는 부모에서 후속 Workflow 실행 사용 |
| 직접 `agent(...,{maxTurns})` | native 직접 옵션에 적용 경로가 없음. 검증된 native 역할 정의의 `maxTurns` 사용. 역할을 복제해 출처/신뢰를 높이거나 API 요청 횟수로 native turn 수를 추정하지 않음 |
| plugin/bundled named resolver 전체 | 로컬 named `.js`와 허용된 `scriptPath`만 지원. native 내부 이름·plugin 출처/우선순위를 추정해 실행하지 않음. 확인된 실제 파일은 scriptPath 사용 |
| source 크기/형식 | 500 KiB 이하의 native Read 전체 텍스트만 허용. 부분/잘린 결과·비텍스트·읽기 거부는 명시적 실패 |
| 구독 backend sampling/output cap | S48 실측의 미지원 판정을 유지. 출력 자르기를 서버 sampling 또는 생성 토큰 상한 구현으로 포장하지 않음 |
| 임의 필터가 모든 통신 경로를 영구 차단 | 전달 성공 보장 불가능. S47/S48의 취소 영수증·프로세스 수거·후속 요청 회복 구현과 구분 |

같은 사용자 권한으로 모든 native 파일과 checkpoint를 동시에 변조하는 공격자를 인증하는
서명 저장소는 아니다. 경로 탈출·변경·누락·불일치 검증과 해당 공격 모델을 구분한다.

## 실패 기록

초기 custom 역할 생략 시험은 native metadata의 빈 model 때문에 거부되었다. 실제 active
turn과 역할 정의는 유효했으며 비교 대상을 수정한 뒤 같은 경로가 통과했다. wrapper 단위
검사의 기존 `agentType` 생략 거부 기대도 변경 계약에 맞춰 수정했다.

첫 재시작 fixture는 native tool output을 문자열로 잘못 가정했고, SDK parent를 completion
이벤트 전에 종료했다. 실제 출력은 text block 배열이다. 명시적 다음 Read 동작 동안 native
completion 이벤트가 전달되는 경로로 수정하고 실제 회수 본문·자식 재실행 0을 확인했다.
이는 실패를 숨기거나 완료 알림을 본문으로 대신한 검증이 아니다.

검수 중 계획 결과가 `const result0 = await agent(`라는 문구를 포함하면 offset 검색을
혼동시킬 수 있는 반례를 추가했다. compiler 코드의 실제 행 시작만 검색하도록 수정했다.

마지막 gateway 검사에서는 역할 기본값 지원 안내와 과거의 명시적 모델 필수 문구를
기대한 검사가 충돌했다. native schema 보존·도구 제한 검사를 유지하고 기대 문구를
현재 역할 기본값/Read 권한 계약으로 갱신했다. 해당 실패 로그도 보존한다.

## 검사·산출물

최초 실제 TUI UUID `2c4f04cf-b9a3-460b-9c84-5d204ec91f81`과 진단 UUID
`9c8ebff5-f169-455a-be48-b6060f5973cd`는 파일 소스 연결 뒤 자식 선택에서 실패했다.
진단 실행의 실제 active 영수증은 Sol/low, Luna/low였다. 테스트 harness의
`--settings.env.CLAUDE_CODE_EFFORT_LEVEL=low`가 native 자식 옵션보다 우선한 것이 원인이다.
제품의 검증을 완화하지 않고 테스트의 부모 기본값을 native `--effort low`로 수정했다.
기존 native 역할 검사도 부모 low와 자식 medium이 다르게 설정된 조건으로 보강했다.
사용자가 전역 effort 환경값을 의도적으로 강제하면 native 모델 변경/자식 effort보다
우선할 수 있다. 다른 effort가 필요한 검증에서는 이 환경값 대신 `--effort`를 사용한다.

자동 검사 결과, 실제 TUI 기록, 최종 개발 바이너리 신원은 이 디렉터리의 `evidence.json`,
`build.json`, `promotion.json`에 기록한다. 사용자 최종 수용 판정은 이 기록과 별개다.

최종 제품 commit: `31ff1184c21d7dac0fccd03394081aacd78b9db5`.
개발 `clauduct.exe` SHA256:
`7feb620944578d85982f8ecac20fe3ffba0096207da0e9b81682f603b5b1036f`.
Go 1.27.1, Windows amd64, CGO_ENABLED=0, `vcs.modified=false`.
세 제품 바이너리를 `D:\AIDEV\clauduct-s36-build`에 반영하고 이전 파일은
`D:\AIDEV\clauduct-s36-build\before-s49-31ff118-20260920`에 보존했다.
설치 release, 전역 설정, 폴더 신뢰, AdGuard 설정은 변경하지 않았다. 필터는 사용자가
활성화했다고 알린 상태에서 수행했으며 임의 필터 전체 호환을 뜻하지 않는다.

- 전체 회귀: 17 packages, 1,668 pass, 3 skip. skip은 별도 콘솔 종료/live/강제 종료 시험.
- gateway race: 567 pass. native Workflow/settings race: 21 pass. vet exit 0.
- 부모 `--effort low`와 다른 역할 effort를 명시한 최종 Workflow 회귀: 105 pass.
- 실제 TUI는 아래 두 프로세스 실행과 원본/재개 journal·status·본문을 대조했다.
  이후 제품 코드 변경은 없고 검증 harness와 문서만 보완했다.

## 최종 실제 TUI

UUID `263ef248-4675-4de2-b905-6d0f2bc4dc1d`, native `2.1.278`.
[본문 없는 검사 기록](inspection-263ef248-4675-4de2-b905-6d0f2bc4dc1d.json),
[기계적 판정](final-evidence.json), [재검사 명령](verify-tui.mjs).

| 관측 | 결과 |
|---|---|
| 파일 source·역할 기본값·pipeline | 실제 scriptPath 실행, public-math Sol/high → `43`, 모델만 지정한 Luna/max → `180` |
| 첫 정상 종료 | API 실패 0, native 도구 실패 0, 결과 미확보 0, checkpoint 저장 2, native 수거 완료 |
| 완료 결과 복원 | `wf_d6781e46-358` → `wf_c46373ab-a6e`, 원본 본문 `43`·`180`, agentCount 0, 과거 자식 backendResponses 0 |
| 중단 계획 복원 | `wf_3d8c5523-f5e` → `wf_c56e9820-7fd`. A `S49_A=303` 재사용, B `started_not_reexecuted`, 새 C `S49_C=144` 한 번만 실행 |
| 원본 변경 여부 | 최초 종료 때의 원본 script/journal identity와 재개 후 journal 해시 대조. 원본 journal 두 개 모두 동일 |
| 중복 재개 | 도구 수준 거부 1, 추가 단계 실행 없음. 다음 일반 답변 `S49_AFTER_DENIAL=53` |
| 재개 프로세스 종료 | API 실패 0, native 도구 실패 0, 결과 미확보 0, 근거 복원 2/실패 0, native 수거 완료 |

후속 원본 재검수에서 B의 OS 명령 실행 근거를 확보하지 못했다. 이전의 “명령 실행 중
TaskStop으로 중단” 표현을 철회한다. 확인된 것은 B child의 시작·취소와 재실행 방지다.
[B 실행 경계 재검수](S44-B-COMPACT-REVIEW.md)를 따른다. C의 `144`를 돌려받아도
B 결과가 없으므로 계획의 `complete:false`를 그대로 유지한다. 이는 재개 기능 실패가 아니라
전체 단계 결과 확보 여부를 정직하게 구분한 값이다.

검수 스크립트 첫 실행은 새 C 결과를 기존 결과의 `body` 필드로 잘못 읽었다. 원래 compiler와
native 저장 결과에서 새 실행은 `value`, 재사용은 `body`임을 확인해 검사만 수정했다.
실제 결과를 바꾸거나 누락된 값을 대신 채운 것은 아니다. 수정된 기계적 검사가 통과했다.

사용자 최종 검증에는 별도 새 UUID로 종료·재시작, 중첩 callback, A/B/C 중단·재개,
중복 거부 후 회복, 모델 변경·수동 압축, 두 Esc 시점, `/context` 조회를 포함한다.
이 사용자 검증 전에는 S49 확대 범위 전체의 최종 수용이나 v0.3.0 출시 합격을 선언하지 않는다.

## 사용자 S44 검수 결과

동일 제품 바이너리의 UUID `97c86a99-71c3-4a4b-b639-05f8fbe30423`를 검수했다.
첫 실행/재개 합계 142건, API 실패 0, native 도구 실패 0, 결과 미확보 0이다.
파일 source, 역할 기본값, 중첩 parallel, native 손자 위임, launcher 재시작 결과 복원,
미시작 C만 재개, 중복 거부 후 47, 두 수동 compact 후 29, 부분 본문 Esc 후 31을 확인했다.
그러나 B는 Bash 결과가 `User rejected tool use`여서 실제 OS 명령 실행 중 중단으로
검증하지 못했고, 본문 전 Esc/초안 복원 및 두 번째 Esc/37은 증거가 없다.
**관측 핵심 기능 합격 / S44 전체 엄격 수용 미완료**로 기록한다.
새 제품 결함을 발견한 것이 아니며 기존 실제 TUI 근거를 취소하는 것도 아니다.
상세 판정·ID·시간·표시 한계는 [S44 사용자 검수](S44-USER-ACCEPTANCE.md),
기계적 대조 결과는 [s44-evidence.json](s44-evidence.json)에 있다.

사용자 후속 설명에서 Esc의 나머지 회복 단계를 의도적으로 생략한 것을 확인했다.
두 번째 compact 요약의 소폭 증가와 native 전후 수치의 계산 범위 차이를 분석했고,
남은 집중 검증 범위는 B의 실제 OS 실행 중 취소·회수다.
[B·compact 집중 검수 및 출시 판단](S44-B-COMPACT-REVIEW.md)을 최신 판정으로 따른다.
