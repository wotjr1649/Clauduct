# S44 사용자 실행 최종 검수 — 2026-09-20

후속 판정: [B OS 보충 TUI 최종 수용](S44-B-OS-ACCEPTANCE.md)에서 남은 OS 실행·회수
근거 공백을 해소했다. 아래 최초 사용자 세션의 관측·미실행 기록은 소급 변경하지 않는다.

사용자의 Esc 단계 의도적 생략 설명과 B/native 코드·compact 수치의 후속 분석은
[B·compact 집중 검수](S44-B-COMPACT-REVIEW.md)에 있다. 아래는 최초 판정의 근거이며,
후속 설명 반영 및 이전 S49 보고의 과도한 실행 주장은 해당 문서에서 정정했다.

**판정: 관측된 핵심 기능은 합격. S44 전체의 엄격한 최종 수용은 미완료.**

새로운 제품 결함이나 예상 밖 API 실패는 이 실행에서 발견하지 않았다. 다만 B의 실제 OS 명령
실행 중 중단, 본문 전 Esc/초안 복원, 두 번째 Esc/37 회복은 이번 증거로 합격 처리할 수 없다.
미충족 항목을 실패 재현이나 제품 결함으로 바꾸지도 않는다. 전체 시험의 최종 합격 또는
확대 범위 전체를 포함한 v0.3.0 출시 합격 선언은 이 공백을 남긴 채 하지 않는다.

## 검수 대상과 신원

- 사용자 UUID: `97c86a99-71c3-4a4b-b639-05f8fbe30423`.
- Claude Code: `2.1.278`. Windows에서 같은 UUID의 첫 실행과 launcher 재시작 실행.
- 제품 source: `31ff1184c21d7dac0fccd03394081aacd78b9db5`.
- 실행 바이너리: `D:\AIDEV\clauduct-s36-build\clauduct.exe`.
- SHA256: `7feb620944578d85982f8ecac20fe3ffba0096207da0e9b81682f603b5b1036f`.
  두 run manifest, session manifest, 검수 시점의 실제 바이너리 해시가 일치한다.
- 전체 root JSONL 247행의 대화·도구 입출력·압축 기록, native Agent 자식 2개와 Workflow 자식
  7개의 대화·도구 기록, Workflow 5개의 상태·journal, 두 status와 native 영수증을 대조했다.
  reasoning 본문이나 관련 없는 profile/자격 증명은 검수 결과에 복사하지 않는다.
- 원본 status/transcript/journal을 수정하지 않았다. 추가 inference, TUI 재실행, 제품 수정,
  바이너리 재빌드, AdGuard 설정 변경, 출시/원격 쓰기는 수행하지 않았다.

[첫 status](tui-97c86a99-71c3-4a4b-b639-05f8fbe30423/first/tmp/clauduct/status-15984.json),
[재개 status](tui-97c86a99-71c3-4a4b-b639-05f8fbe30423/resume-1789910155315/tmp/clauduct/status-22904.json),
[메타데이터 검사](inspection-97c86a99-71c3-4a4b-b639-05f8fbe30423.json),
[재현 가능한 증거 대조](review-s44.mjs), [판정 JSON](s44-evidence.json).

검사 실행:

```powershell
node verification/workflow-completion-20260920/review-s44.mjs audit
```

이 명령의 exit 0은 아래 관측 사실과 미검증 항목의 분류가 저장된 근거와 일치한다는 뜻이다.
새로운 실시간 기능 시험 또는 S44 전체 합격을 뜻하지 않는다. JSON은
`observedCore: pass`, `strictS44Acceptance: incomplete`를 함께 기록한다.

## 실행 전체 상태

| 항목 | 첫 실행 | 재개 실행 | 합계/판정 |
|---|---:|---:|---|
| 요청 | 89 | 53 | 142 |
| API 실패 | 0 | 0 | 0 |
| native 도구 실패 | 0 | 0 | 0 |
| 최근 결과 미확보 | 0 | 0 | 0; journal·부모 본문으로도 대조 |
| 요청 취소 | 1 | 1 | 서로 다른 취소 원인 |
| 의도한 Workflow 중복 거부 | 0 | 1 | 기대한 부정 시험 |
| Workflow 근거 저장/복원/실패 | 3/0/0 | 4/2/0 | 복원 실패 없음 |
| 프로세스 종료 | exit 0 | exit 0 | native 수거 확인, watchdog 강제 종료 없음 |

첫 실행 seq 86 취소는 종료 직전 Luna/high **auxiliary** 요청이며, main Esc의 증거가 아니다.
별도로 B child의 native `aborted` 영수증이 있다. 재개 실행 seq 25는 Terra/high main의
사용자 취소다. 두 status 모두 예상 밖 거부/전달 실패가 없으며, 취소의 실제 근거가 있다.
단일 세션 결과로 모든 환경의 무결점이나 flaky 발생 확률 0을 주장하지 않는다.

## 단계별 실제 결과

| 항목 | 실제 근거 | 판정 |
|---|---|---|
| 파일 Workflow | root 37행의 `scriptPath`, native Read 영수증, 원본 journal의 `43`, `180` | 합격 |
| 역할 기본값 | public-math 자식 `a79f754b8453603b7`, model/effort 미지정, 확정 Sol/high | 합격 |
| 모델만 지정 | 자식 `a605349b6b3db64db`, Luna 지정/effort 생략, 확정 Luna/max | 합격 |
| 중첩 parallel callback | LEFT `56`, RIGHT `81`; journal에서 두 started 뒤 첫 result; 실제 시간도 겹침 | 합격 |
| Agent가 Agent 호출 | 중간 Terra/medium → 미지정 손자에 Terra/medium 상속, `144` 확보 후 `145` | 합격 |
| 완료 결과 launcher 재시작 복원 | 저장된 `43`/`180` 회수, 새 agent 0, 원본 자식들의 재개 프로세스 backendResponses 0 | 합격 |
| 중단된 독립 계획 재개 | A `S44_A=303` 재사용, B `started_not_reexecuted`, 새 C `S44_C=144` 한 번 | 합격; B의 OS 명령 실행 중 취소까지 입증된 것은 아님 |
| 중복 재개 차단 | 한 차례 native PreToolUse 거부, 추가 Workflow run/child 없음, 다음 `S44_AFTER_DENIAL=47` | 합격 |
| 모델 전환·수동 압축 | 두 manual compact 성공, 두 번째 직전 일반 입력 없음, 다음 `S44_AFTER_COMPACT=29` | 합격 |
| 부분 본문 Esc 후 회복 | 부분 문장 수신 → CANCELLED → 같은 세션에서 `S44_RECOVERY_A=31` | 관측된 한 시점 합격 |
| 본문 전 Esc/초안 복원 | 해당 시점을 증명할 기록 없음 | 미검증 |
| 두 번째 Esc/37 | 두 번째 `S44_ESC` 제출 및 `S44_RECOVERY_B=37` 기록 없음 | 미관측 |

원본 및 재개 ID:

| 용도 | runId | taskId |
|---|---|---|
| SOURCE_RUN | `wf_d99eac77-3dd` | `wwdewjz9w` |
| 중첩 parallel | `wf_46291b6d-602` | `w2yv2o9b2` |
| PLAN_RUN | `wf_0b6bb6ef-b30` | `wtpj1d1e2` |
| SOURCE 결과 복원 | `wf_9ccb2c8a-67c` | `wc09noh48` |
| PLAN 미시작 단계 재개 | `wf_dc42ba37-87b` | `wr41z6g1x` |

중간 Agent `a27b2f629c105edda`는 손자 `a2c287b1305e60298`를 한 번 호출했다. 손자의 실제
`144` 본문이 13:12:24.318Z 완료 알림으로 전달되고, 중간의 `145` 보고는 13:12:30.087Z다.
status의 원래 요청 필드 존재 판정은 model/effort 모두 false, `delegation-inherited`다.
native transcript의 변환된 `model: sonnet` 및 native 기본 effort low를 backend 확정 선택으로
오해하지 않는다. 해당 자식의 확정 선택과 backend usage 경로는 Terra/medium으로 연결된다.
주기적 조회/SendMessage를 통한 결과 회수는 없었고, parent_wait 조건 관측은 첫 실행 2건이다.
자연 발생 empty_reply_wait는 이번 실행 0건이므로 별도의 성공 근거로 추가하지 않는다.

## B 중단의 정확한 범위

시간은 UTC이며 한국 시간은 +9시간이다.

1. 13:13:53.045 — A가 `S44_A=303` 반환.
2. 13:13:58.729 — B가 Bash 도구를 요청. 명령은 STARTED 출력 → 90초 대기 → FINISHED 출력.
3. 13:14:17.361 — 부모가 `TaskStop(wtpj1d1e2)` 호출.
4. 13:14:17.385 — TaskStop 성공.
5. 13:14:17.413 — B의 도구 결과 `User rejected tool use`.
6. 13:14:17.426 — B native `turn_aborted`, `permissionRequests: 1`.

`S44_B_STARTED`는 명령 문자열에만 있고 **실행 결과로 출력된 증거가 없다**. B Agent가 시작된
것과 PowerShell 프로세스가 실제 실행된 것은 다르다. 이번 실행은 도구 승인 대기 중 중단된
흐름과 일치한다. Bash 요청만으로 실행 중 프로세스 수거를 합격 처리하지 않는다.

재개 시 B를 자동 반복하지 않은 것은 요구한 보수적 정책을 만족한다. 새 C는
`a0f2a6224128db266` 한 명이며, A/B의 과거 ID에 새 backend 응답이 없다.
B 본문 부재로 재개 결과의 `complete:false`가 유지됐고 부모도 이를 설명했다.
이 값은 잘못된 실패가 아니라 전체 단계 본문 미확보를 정확히 나타낸다.

## 압축·Esc·context

실제 압축 순서는 root 157–216행에 있다.

- Sol/high 선택 → 일반 응답 `S44_BEFORE_COMPACT=17` → 첫 `/compact`.
- 첫 compact: 13:23:54.064–13:24:53.527Z, native duration **59.365초**.
- Terra/high 선택 → **일반 프롬프트 없이** 두 번째 `/compact`.
- 두 번째 compact: 13:25:11.945–13:25:57.331Z, native duration **45.287초**.
- 이후 Terra/high에서 `S44_AFTER_COMPACT=29`.

사용자 제출의 “보냈다”와 달리 두 번째 직전 일반 입력은 없다. 압축 사이의 user-role 요약은
`isCompactSummary:true`인 native 생성 기록이므로 사람의 새 일반 입력으로 세지 않는다.
두 압축 모두 backend 경로 Sol/high인 것은 이전 확정 모델·effort로 먼저 압축하는 구현과
일치한다(`go/internal/gateway/context.go:197` 및 284). Terra 일반 생성은 압축 후 적용됐다.
두 건 모두 `trigger:manual`이며 모델 선택만으로 자동 압축된 사례는 없다. 239K/450K 경계
자동 압축은 이번 입력량에서 발생하지 않았다. 수동 압축 성공으로 경계 시험을 대체하지 않는다.

Esc는 root 225–234행에 한 번만 기록됐다. 13:26:57.351Z에 첫 문장의 일부가 저장되고
13:26:57.367Z에 사용자 중단, 다음 13:27:13.338Z에 `S44_RECOVERY_A=31`이 반환됐다.
seq 25는 text delta 13개, first byte부터 취소까지 384ms다. 이는 전달된 부분 응답과
회복의 근거이며 **실제 화면 그리기 시각 또는 입력창 초안 복원**의 근거는 아니다.
취소 시 동일 부분 문장이 transcript에 두 번 나타나는 것을 두 backend 실행으로 세지 않는다.
해당 `S44_ESC`의 서버 도달 제출은 한 번이며, 두 번째 제출과 37 회복은 기록에 없다.

| `/context all` | 1회 | 2회 | 3회 |
|---|---|---|---|
| 최초 총량 | 9.3K | 9.3K | 9.3K |
| 최초 Messages | 19 | 14 | 14 |
| 마지막 총량 | 13K | 13K | 13K |
| 마지막 Messages | 3.9K | 5.2K | 6.5K |

마지막 Messages 증가 및 공통 500K 표시는 이미 수용한 native 표시 한계가 그대로 관측된 것이다.
표시 행까지 완전히 고쳤다고 주장하지 않는다. 마지막 조회 이후 일반 생성은 없으므로 이 세
출력의 다음 backend 입력 제외 여부를 이 실행만으로 새로 실증했다고도 말하지 않는다.
일반 생성 사전 계수는 두 실행 모두 0, 별도 count_tokens는 총 58건 성공이다.
commandRun에는 context/all만 있고 키 입력·화면 표시 시각은 없으므로 정확한 TUI 지연은
확정 불가다. status의 개별 countMs를 전체 화면 표시 지연으로 대체하지 않는다.

## 최종 남은 일

이번 로그에서 수정이 필요한 새 제품 결함은 발견하지 않았다. S44 전체를 엄격하게 닫으려면
전체 시나리오를 반복할 필요 없이 다음 조건만 확인하면 된다.

1. B의 공개 명령이 승인되어 실제 STARTED 출력을 낸 뒤 TaskStop; 이후 명령 종료·회수와
   A/B 재실행 0, C만 실행을 대조한다. 승인 대기에서 중단한 것을 이 조건으로 세지 않는다.
2. 본문 전 Esc의 초안 복원/삭제 후 다음 대화, 본문 후 별도 Esc 및 37 회복을 각각 확인한다.
   화면·초안은 사용자 관측으로 기록하고 backend 취소/후속 응답과 대조한다.

모든 멀티모달·필터·강제 종료 환경, 자연 발생 빈 응답, 자동 압축 경계, 이번에 사용하지 않은
WebSearch/non-streaming/Agent resume까지 S44로 합격 범위를 넓히지 않는다. 기존 검증 이력과
명시한 지원 범위를 유지한다. 사용자가 이미 수용한 표시·환경 한계는 새 차단 사유로 추가하지 않는다.

검수용 스크립트의 최초 실행은 native user-role 압축 요약을 일반 입력으로 세어 실패했다.
해당 행의 `isCompactSummary:true`를 확인하고 입력 분류만 수정했으며 원본 기록은 그대로다.
수정 후 기록 대조가 통과했다. 이 검수 도구 오류는 제품의 flaky 동작으로 분류하지 않는다.
