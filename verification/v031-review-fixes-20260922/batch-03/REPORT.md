# v0.3.1 리뷰 수리 batch-03 — 1–4 실행 사이클

2026-09-22, `D:\AIDEV\clauduct-v031`, branch `fix/v031`.
기준 HEAD는 `149068edd693fb860a03244a2ea15764bcd68c34`이고 기존 v0.3.1 변경 위에 수리했다.

**수정 범위의 회귀·실제 작업 검증은 PASS, 전체 무결함·출하 판정은 HOLD다.**
정상 SDK 작업 중 한 번 발생한 HTTP 502의 최초 오류명이 당시 검사 로그에서 유실됐다.
현재 후보의 후속 실검증 PASS만으로 그 실패를 해결했다고 판정하지 않는다.
별도 native Claude 재리뷰도 결과를 받지 못했다. 직접 코드 검토와 아래 검사는 완료했다.

**1–4 실행 결과**

| 순서 | 실행한 작업 | 판정 / 근거 |
|---|---|---|
| 1 | A1-03: 다른 Agent가 pending이어도 해당 자식의 실제 Workflow 증거를 조회. A3-03/INT-02: 거부 이전에 검증된 출처를 보존하고 continuation에도 Workflow 검증 이력으로 집계 | 정상·거부·가짜 역할 이름·자식 결과 추적 대조 PASS. `workflow-before.txt`, `workflow-after.txt`, `review-unrelated-accepted.txt` |
| 2 | A1-02/INT-05: 복합 short 옵션의 값 경계를 공통 scanner에서 처리. A1-05: 손상된 plugin 역할의 불확실성을 해당 namespace에 한정. A3-01: optional count와 생성의 compact 분류 일치 | 실제 native argv 대조와 63개 공개 옵션 arity, 우선순위·다른 namespace, 실제 usage 캐시 재사용 PASS. `compat-after.txt`, `count-accepted.txt`, `hold-app.txt` |
| 3 | A5-03: CI에 runtime_evidence/policy_evidence vet와 오프라인 검수기 추가. A5-04: feature counter는 그대로 두고 누계 CANCELLED만 0인 반례 추가. A5-01: 과거 hash 대조와 판본 제한 기록 | 로컬 CI 대응 명령 PASS. 취소 assertion 제거 시 FAIL. 과거 raw TCP 원본은 복구 불가로 명시. `static-final.json`, `mutation-results.json`, `historical-provenance.json` |
| 4 | 최종 제품 수정 뒤 전체 일반·race, 태그 검사, 실제 backend 파일 도구·Agent/Workflow·손실 복구·TUI, 직접 리뷰 | 범위별 PASS. 미분류 502와 Claude 재리뷰 미수신 때문에 전체 HOLD. `REVIEW.md`, `evidence.json` |

직접 검토 중 새로 확인한 경계 결함도 같은 사이클에서 수정했다. 첫 수정은
중첩된 일반 Agent가 `workflow-subagent`라는 이름을 쓰면 root 전용 Workflow 조회로
거부될 수 있었다. parent가 있는 일반 Agent는 자기 sidecar를 기다리도록 했다.
또한 무관한 Workflow journal의 손상이 일반 Agent의 늦은 sidecar보다 먼저 거부를
발생시켰다. 대체 Workflow 증거가 성공한 경우에만 채택하고, 일반 Agent는 자기 증거로
검증한다. 증거가 끝내 없으면 기존 1초 상한 안에서 거부한다.
`TestWorkflowNamedAgentWaitsForItsOwnMetadata`와
`TestPendingAgentDoesNotInheritUnrelatedWorkflowReadFailure`가 이를 검사한다.
초기 후자 fixture의 astra alias를 opus로 잘못 지정한 실패도 보존했고, 올바른 fable
fixture와 수정 제거 mutation에서 최종 원인·판별력을 확인했다.

**유지한 정책**

- #42: 정상 정의와 우선순위 보존, 불확실한 역할만 거부.
- #45: native 모델 선택을 유지하고 실제 요청의 model/effort를 검증하며 자식 완료를 추적.
- #50: 압축 직후 이전 입력량을 하한으로 이월하지 않음. 실제 미디어 초과 관측 뒤 재검토.
- #56: 자동 compact만 medium 상한, 이후 원래 effort. 수동 compact 정책 유지.
- 일반 생성·압축에 원격 사전 계수를 추가하지 않음. 실제 backend usage가 권위 있는 수치.
  새 optional count 회귀도 inference 1회 / Count 0회 / backend-usage 재사용을 검사한다.
- projects 루트 실패를 역할 부재로 취급하지 않는 기존 #48 정책 유지.
- 모르는 model/effort 거부, native 자동 재시작 금지, 이미 실행됐을 수 있는 요청 재실행 금지.

**관측된 검사**

Windows, Go 1.27.1, Claude Code 2.1.278.
일반 빌드 CGO_ENABLED=0, race만 1과 기존 gcc를 프로세스 PATH에서 사용했다.
보호 설정은 변경하지 않았다. 보호 On은 사용자의 마지막 확인 상태이며 UI 재측정은 아니다.

| 검사 | 관측 |
|---|---|
| 전체 일반 | 18/18 packages PASS. `go-test-accepted.txt`; app 533.456s, gateway 98.628s |
| 전체 race | 18/18 packages PASS. `go-race-accepted.txt`; app 566.912s, gateway 146.303s |
| 정적·태그 | gofmt 출력 0, vet/build/runtime_evidence vet/policy_evidence vet 모두 exit 0. `static-final.json` |
| 오프라인 검수기 | 최상위 3개와 TUI 대조 16개 PASS. 16개 중 정상 대조 2개. `offline-validators.txt` |
| 수정 제거 | 9/9에서 의도한 assertion FAIL, compile 실패 0. `mutation-results.json` |
| HOLD 재현 | native help 63개, subcommand 경계, 실제 파일 게시 실패·재발행, 최신 미완료 receipt 거부·다음 발행 복구 PASS |
| 문서 인용 | 추적 Markdown 128개, 인용 89개, 내부 링크 725개, 실패 0. `doc-citations.txt` |
| 검토·복구성 | 20개 원본 backup hash 일치, 실제 변경 19개, `git apply --reverse --check batch.patch` PASS, `git diff --check` exit 0 |

일반·race 최종 실행은 동시 실행했고 실제 backend 검사와 일부 겹쳤다. 느려진 실행 시간은
성능 개선이나 회귀의 증거로 사용하지 않는다. 최종 제품 소스는 그 이후 바꾸지 않았다.
`candidate-source.json`은 현재 Go/embedded JS/module 입력 261개를 고정한다.
`before.json`, `after.json`, `batch.patch`는 이 사이클의 변경만 구분한다.
7개 제품 Go 파일, 7개 테스트 파일, CI 1개, 문서 4개를 변경했다.
기존 staged/unstaged/untracked 작업은 그대로 보존했고 설치·commit·push·배포는 하지 않았다.

**실제 backend: 파일 도구·자식·응답 손실**

`live-tools-diagnosis.txt`: PASS, 60.43s, native PID **27120**, 시작 1회,
실호출 **13회**, exit 0, 제품 cleanup 성공.

1. native Write로 공개 파일 작성 후 Read로 확인.
2. 실제 Agent 1개와 Workflow 1개 실행. 두 자식 결과가 실제 다음 부모 입력에
   들어간 `parent_received=2`를 기다림. 모델이 출력한 완료 문구만 믿지 않음.
3. 두 번째 파일의 Write 성공을 디스크에서 확인한 뒤, 그 다음 모델 응답을
   gateway socket에서 한 번 끊음. native의 같은 요청 재전송은
   `NATIVE_REQUEST_REPLAY_BLOCKED`; 해당 손실 단계 backend 실행은 정확히 1회.
4. 같은 PID에서 새 요청으로 파일을 Read하고 앞선 두 자식 식별자와 복구 마커 회수.
   root transcript의 Write 호출/성공 2/2, Agent 호출 1, Workflow 호출 1을 확인.

검증 overlay는 실제 Direct transport를 사용한다. connection overlay는 metadata 계측과
명시적인 1회 socket 손실 주입만 추가하며, 제품 종료 처리 알고리즘을 대체하지 않는다.
공개 합성 프로젝트·격리 profile·허용 도구만 사용했다. run당 상한 18회, native 6분,
Go 7분이다. 고의 손실에서 발생한 실패와 재전송 차단은 기대 결과로 명시하여 판정했다.

**실제 backend: native TUI**

`tui-final.txt`: PASS, **77.03s**, PID **276**, 시작 1회, 실호출 **5회**,
exit 0, cleanup 성공. 기존 TUI 검수기의 누계와 실제 native transcript 판정을 그대로 사용했다.

초기 사실 기억 → `/compact` → code/color/release 사실 회수 → 실제 text delta 이후 Esc →
같은 프로세스 복구 응답 → `/exit` 순서다. 생성 완료 3, compact 완료 1,
생성 취소 1 및 전체 CANCELLED 1을 모두 확인했다.
완료 판정은 화면 마커가 아니라 transcript의 사실·세션·순서와 제품 누계다.
TUI 계측 마커는 드라이버의 다음 입력 시점만 결정한다.

**초기 실패를 보존한 이유와 처리**

| 로그 | 실패 / 처리 |
|---|---|
| `live-tools-attempt1.txt`–`attempt3.txt` | SDK의 빈 결과·이전 작업 완료 통지를 새 단계 결과로 잘못 읽은 검증 드라이버 결함. 실제 자식 결과 2개가 부모 입력에 포함될 때만 다음 단계로 이동하도록 보완 |
| `live-tools-attempt4.txt` | 네 단계는 완료했으나 잘못된 Agent 인자가 한 차례 거부되어 정확한 호출 수 assertion 실패. 공개 prompt에 필수 description 등 완전한 인자를 명시. assertion은 유지 |
| `live-tools-attempt5.txt` | 초기 후보 PASS, PID 7708, 13회. 이후 추가 코드 수리 때문에 최종 후보의 대체 근거로 사용하지 않음 |
| `live-tools-final.txt` | 최종 후보 정상 Agent/Workflow 단계에서 HTTP 502 후 재전송 차단. **원인 미확정, 별도 HOLD** |
| `live-tools-diagnosis.txt` | 종료 시 최초 제품 오류명·StreamEnd를 보존하도록 드라이버 보완 후 같은 후보 PASS. 이전 502를 종결하는 근거는 아님 |
| `tui-attempt1.txt` | go test 중계로 stdin을 잃어 native 시작 오류, 실호출 0. PTY에서 컴파일된 검사 바이너리 직접 실행으로 수정 |
| `tui-attempt2.txt` | PowerShell의 dotted flag 해석 오류. 검사 인자를 하나의 문자열로 인용하여 수정. 실제 검사는 실행되지 않음 |
| `tui-attempt3.txt`, `tui-attempt4.txt` | 긴 마커 줄바꿈·수동 제어 지연으로 종료 명령이 늦음. 기능 응답은 관측했지만 5분 deadline 실패. 짧은 마커와 연속 자동 입력으로 수정하며 deadline·최종 assertion 유지 |
| `tui-final.txt` | 변경한 드라이버가 정상 종료까지 수행하고 기존 검수기 PASS |

전체 integration 시도의 관측된 backend 호출 합계는 **85회**다(SDK 70 + TUI 15).
성공한 최종 두 실행만 합치면 18회지만 이것을 전체 사용량으로 표시하지 않는다.
별도 Claude 재리뷰 요청의 실제 사용량은 결과 미수신으로 확인하지 못했다.

**현재 남은 경계**

1. **미분류 HTTP 502 — 제품 전체 판정을 막음.** PID 20456, 정상 단계의 main step 2,
   body 58,150 bytes. 고의 손실 0, trace write error 0, client cancellation false인 상태에서
   connection 9가 HTTP 502 응답 186 bytes를 썼다. 같은 turn/step/body 재요청은 connection 2에서
   400 재전송 차단을 받았다. local write reset이나 잘못된 중복 판정이라고 단정할 증거가 없다.
   wire 길이만으로 EMPTY_REPLY와 INVALID_SSE 등을 구분할 수 없다.
   최초 category와 stream 종료 근거를 보존하지 못한 실행은 소급 복구할 수 없다.
   `unresolved-live-502.json`과 원래 trace를 유지한다.
2. **독립 Claude 재리뷰 — NO_RESULT.** Clauduct를 거치지 않는 native firstParty,
   현재 선택 opus, 요청 effort xhigh, tools 없음, 격리 profile로 12개 파일의 검토 payload를
   전달했다. 6분 안에 결과가 없어 process tree를 종료하고 WaitForExit를 확인했다.
   PID 18932 및 직접 자식의 후속 read-only 조회도 0건이다. 동일 리뷰를 재발송하지 않았다.
   payload는 마지막 무관 Workflow 오류 보완 전 후보이므로 결과가 있더라도 최종 전체 승인이
   될 수 없었다. 현재 후보 직접 검토는 `REVIEW.md`에 기록했다.
3. **과거 raw TCP 원본 — 복구 불가.** Node·.NET hash는 당시 기록과 일치하지만 raw TCP는
   현재 파일과 다르며 .tmp 후보 109개에서도 원래 hash를 찾지 못했다.
   과거 결과를 새 hash로 덮지 않고 원본 보고서에 독립 재현 제한을 명시했다.
   이는 현재 제품 검사 결과와 별도의 역사적 증거 제한이다.

자동 승인 검토가 task 임시 Claude 리뷰 profile 삭제를 `blocked by policy`로 거부했다.
세부 규칙은 제공되지 않았다. 다른 도구·경로로 삭제를 재시도하지 않았으며
`.tmp/integration-temp/batch03-native-review`가 남는다.
정상 integration 테스트의 profile 정리 성공과 이 별도 잔여물을 구분한다.
`claude-review-cleanup.json`에 기록했다.

**후속 순서**

먼저 최초 오류를 보존하는 보완된 probe에서 502가 다시 관측될 때 category·StreamEnd로
최소 재현을 확정한다. 확인한 원인 위치만 수정하고 같은 후보의 관련 회귀·실호출을 다시
검사한다. 최초 오류를 얻지 못한 상태에서 replay 보호를 완화하거나 성공 횟수를 채우기 위한
반복 실행은 하지 않는다. 그 뒤 최종 소스 manifest를 기준으로 독립 Claude 리뷰 결과를
받아 통합 판단한다. 외부 필터 설정 변경·자동 native 재시작·불확실한 도구 재실행은 해법이 아니다.

독립 Node 31개·.NET 35개·raw TCP는 이 사이클에서 재실행하지 않았다. 이미 정한 제품 실제
동작 기준에 따라 환경 진단으로 별도 보존하며 기존 FAIL/HOLD를 PASS로 바꾸지 않는다.
현재 한 사이클의 코드 수리·직접 리뷰·범위별 검증 수행과 전체 Success는 서로 다른 판정이다.
