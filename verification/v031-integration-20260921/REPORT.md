# v0.3.1 통합 검수 — SDK 출력 결함과 잔여 범위

2026-09-21–22. 개발 worktree `D:\AIDEV\clauduct-v031`, `fix/v031`의 앞선 여섯 묶음을
보존하고 실제 native와 구독 backend를 연결했다. 아직 미커밋·미출하이며 설치본은 바꾸지 않았다.

## 실제 실행에서 발견한 출력 결함

Luna/low로 공개 합성 역할 `Fork`와 `public-peer`를 병렬 실행했다. 두 자식의 확정 선택과
부모 수신은 확인됐고 backend도 부모 답변 텍스트를 보냈지만 native `-p` stdout은 줄바꿈
1 byte뿐이었다. 세 번의 초기 실호출에서 같은 실패가 발생했다. 세 번째에는 본문을
출력하지 않고 backend text delta byte 수를 계측하여, 마지막 답변 66 bytes가 backend에서
도착한 사실을 확인했다. 모델 호출을 더 반복하기 전에 로컬 최소 재현으로 전환했다.

[native 회귀 검사](../../go/internal/app/native_print_test.go)에서 텍스트만 보낼 때는 출력되고,
같은 텍스트와 opaque reasoning을 함께 보내면 exit 0과 빈 출력이 나왔다. 변환기가 텍스트를
먼저 스트리밍하고 완료 때 `redacted_thinking`을 뒤에 추가하는데, native SDK/print는 마지막
assistant 블록을 최종 출력으로 취급하는 것이 원인이다. 이전 검사는 reasoning 없는 응답으로
화면 출력을 검사하거나, 결과 추적 성공만 검사하여 이 조합을 놓쳤다.

[relay](../../go/internal/gateway/messages.go)는 검증된 native step의 `mode=sdk`일 때 기존
Workflow용 완료 시 텍스트 전달 경로를 재사용한다. opaque reasoning을 보존하면서 최종
텍스트를 마지막 블록에 둔다. TUI의 점진적 텍스트 전달과 tool delivery barrier는 유지한다.
검증·usage·빈 응답 거부를 제거하지 않았다.

수정 뒤 로컬 두 대조군이 PASS했고, SDK 조건만 제거하는 Go overlay mutation은 다시
`stdout_bytes=1`의 assertion 실패로 검출됐다. mutation 결과는 컴파일 실패가 아니다.

## 실제 backend 결과와 요청 원장

설치된 Claude Code 2.1.278, Go 1.27.1 Windows에서 실행했다. 실제 검증은 Luna에 한정하고
전체 요청 상한을 40회로 선언했다. 최종 사용은 **40회: 생성 37회, 계수 3회**다.
자격 증명은 제품 provider가 읽었으며 runtime/home/TLS/redirect 검사를 유지했다.
고정 공개 합성 입력과 격리한 native profile/project만 사용했다. SDK에는 위임 도구만
제공하고 TUI 도구는 비웠다. 사용자 MCP·개인 프로젝트·설정은 섞지 않았다.
전체 본문과 자격 정보는 보고서에 저장하지 않았다.

| 실행 | 요청 수 | 관측 |
|---|---:|---|
| 수정 전 native 병렬 위임 3회 | 5 + 5 + 5 | 매번 자식 수신 2개, native 최종 출력 비어 실패 |
| 수정 후 위임 + 재시작 첫 검사 | 6 | 빈 출력 해결. 재시작 답변은 있으나 식별자 요청이 기대한 PUBLIC 표식과 불일치 |
| 재시작 프롬프트의 PUBLIC 표식 복사 요구를 명시한 최종 검사 | 6 | 역할 두 개의 선택 확인·부모 수신 2개·stdout 표식 4개·재시작 후 표식 4개 모두 PASS, 26.695초 |
| 동일 JPEG 3개의 user/tool 위치 대조 | 4 | user 3,497 = 3,497. tool 2,828 ≠ 3,525, FAIL |
| 긴 합성 이력의 자동 압축 경로 | 4 | 입력 37,159토큰 계수 = usage, 필수 식별자 4개 보존, high → medium → high, PASS |
| 최초 native TUI 시작 | 0 | 새 임시 폴더의 신뢰 등록에서 승인 대기. 5분 상한 후 회수·정리 완료 |
| 승인 후 실제 TUI, 2026-09-22 | 5 | 수동 압축·사실 보존·Esc 중단·같은 세션 복구 관측. native exit 0, cleanup 성공. 취소 경로를 잘못 센 검수 assertion은 FAIL 보존 |

[native 실호출 검사](../../go/internal/app/integration_runtime_evidence_test.go)는 build tag
`runtime_evidence`, `CLAUDUCT_EVIDENCE_LIVE=1`로만 실행된다. 최종 위임·재시작 검사의
ledger는 Luna/low 8회 상한이다. 초기 실패를 성공으로 덮지 않았고 추가 지출도 원장에 포함했다.

[긴 압축 검사](../../go/internal/gateway/compaction_runtime_evidence_test.go)의 실측은
일반 생성 28/19, 압축 37,159/144, 이후 생성 28/20 input/output tokens다. 두 route의
ledger가 각각 2회로 제한된다. 이는 gateway에 공개 합성 압축 요청을 넣은 검사이며,
native가 threshold에 도달하여 자동 압축한 TUI 실측이나 모든 요약 품질의 보장은 아니다.

## 남은 결함·호환성 경계

**도구 결과 이미지 계수:** [기존 실측](../release-investigation-20260920/REPORT.md)의
입력 위치 차이가 [Luna 대조](../../go/internal/upstream/mixed_pdf_evidence_test.go)에서도
재현됐다. 현재 차이는 697토큰으로 과거 Astra의 1,418과도 다르다. 고정 보정값은 해결이
되지 않는다. 같은 encoded 입력의 warmup과 생성 usage가 다르며 provider 내부 구현은
확정하지 않았다. 기존 tool-output image/file의 정확 계수 미지원과 일반 생성의 독립 처리를
유지했다. 실패한 opt-in assertion도 유지한다.

**소켓 종료:** AdGuard와 AdguardSvc가 실행 중인 상태에서 설정을 바꾸지 않고
[raw loopback 대조](../../go/internal/gateway/socket_runtime_evidence_test.go)를 실행했다.
즉시 종료 200회, 수신 ACK 뒤 종료 200회 모두 실패 0이었다. 이전 실험의 RST 원인을
해결했다는 판정은 아니다. 현재 payload·실행 조건에서 재현되지 않았으며 HTTP/TUI 전달
완료 전체의 증거도 아니다. 검증되지 않은 linger/drain 변경을 제품에 넣지 않았다.

**v0.3.0으로 되돌린 세션:** 기준 commit `149068e`를 별도 task worktree에 체크아웃하고
변경하지 않은 실제 구형 `loadChoice`를 실행했다. 기존 합성 journal은 수용했고,
`customRole:true` 또는 `source:native-selection`만 추가/변경한 대조군은 각각 거부했다.
이 검사의 PASS는 안전한 거부의 확인이다. 같은 세션의 다운그레이드 재개 지원이 아니다.
[되돌리기 문서](../../docs/v2/PACKAGING.md)에 새 세션 시작 또는 v0.3.1 유지 조건을 기록했다.
실제 사용자 journal은 읽거나 수정하지 않았다. [실행한 probe](compat-v030-probe_test.go)는
구형 worktree의 `go/internal/gateway/`에 두고 `go test -run '^TestV031ChoiceDowngradeBoundary$'`
로 실행했다. 세 대조군 모두 예상한 판정으로 PASS했다.

**#50/#52:** 미디어 포함 압축 후 최초 길이 초과는 이번에도 실제 관측하지 못했다.
#50 현행 추정·카운터 유지와 #52 junction 경계의 wontfix는 변경하지 않았다.

## 로컬 검사

| 검사 | 결과 |
|---|---|
| 전체 `CGO_ENABLED=1 go test -count=1 -timeout=12m -race ./...` | PASS. app 305.547초, gateway 24.694초 |
| 실제 native 출력의 텍스트/텍스트+reasoning 대조 | 수정 전 후자 FAIL, 수정 후 2/2 PASS, 4.757초 |
| SDK 수정 제거 mutation | assertion으로 검출, 3.138초 |
| `gofmt -l .`, `go vet ./...`, `go build ./...` | PASS |
| Node V1 | 96개 파일을 32/32/16/16으로 분할 실행. 합계 130 PASS, 1 skip, 실패 0 |
| verification/poc의 정상 실행된 별도 Node 검사 | 9개 파일 PASS |
| PowerShell runner·합성 installer 검사 | PASS. runner 환경/경로/실패/timeout 회수, installer 26개 검사 |
| TUI 검수 수정 후 취소·실패 분류와 누적 진단 | 6개 검사 `-race` PASS, gateway 1.191초. 일반 취소·실패의 누적 카운터와 eviction 뒤 집계 assertion 추가 |
| TUI 검수 수정 후 opt-in 코드 컴파일·vet | `go test -tags=runtime_evidence -run '^$' ./internal/app` 컴파일 PASS, 테스트 실행 0개. app/gateway tagged vet PASS |
| 문서 인용·최종 diff | 문서 122개, 인용 89개, 로컬 링크 676개, 실패 0. `git diff --check` PASS |

Node 전체 첫 실행은 하네스의 60초 상한에 걸려 종료됐다. 종료 후 task native/Node 잔여
프로세스가 없음을 확인하고 작은 묶음으로 나눴다. 마지막 32개 묶음도 전체 상한에 걸려
16개씩 분리한 뒤 완료했다. skip 1개는 기존 `test-rate-limit-observation.mjs`가 명시한
`BLOCKED_NOT_RUN_DEBUG_RUNTIME_UNSUPPORTED` factory probe다. verification/poc에 처음 `--test`를
잘못 적용한 시도에서는 PATH 없는 하네스 때문에 문서 검사의 git 조회가 실패했고,
`test-http-transport.mjs`와 `test-dotnet-http-transport.mjs`는 실행 인자를 runtime guard가
`DEBUG_RUNTIME_UNSUPPORTED`로 거부했다. 두 거부 검사는 다른 실행 경로로 재시도하거나
guard를 약화하지 않았으며 **NOT_RUN**이다. 나머지 9개는 문서화된 standalone 경로로
실행했다. 문서 검사의 git 조회 실패는 정상 PATH에서 해결됐다.

## 승인 후 실제 TUI 검수와 검수 코드 수정

최초 실행은 신뢰 등록 승인 전 5분 상한으로 종료됐으며 native exit 1, cleanup 성공,
backend 0회였다. 이 기록은 [요청 원장](evidence.json)에 보존했다.
2026-09-22 사용자가 `D:\AIDEV\clauduct-v031\.tmp\integration-temp` 아래 공개 합성 프로젝트를
해당 임시 profile에 신뢰 등록하도록 승인했다. 실제 PTY에서 그 범위의 새 프로젝트만 등록했다.

Claude Code 2.1.278, Luna/low의 같은 TUI에서 다음을 관측했다.

1. 최초 응답에 `PUBLIC_TUI_47`이 출력됐다.
2. `/compact`가 완료됐고, 다음 응답이 `PUBLIC_TUI_47`, `blue`, `47`,
   `PUBLIC_COMPACT_DONE`을 모두 출력했다. gateway의 compaction 성공 기록은 1개다.
3. 공개 합성 긴 목록 생성 도중 Esc를 보냈다. native는 `Interrupted`를 표시했고 gateway는
   해당 main generation을 `outcome=broken`, `category=CANCELLED`로 기록했다.
4. 같은 세션의 다음 요청에서 `PUBLIC_TUI_RECOVERED`를 출력했다. `/exit` 후 native exit 0,
   cleanup 성공, 임시 profile/project 제거를 확인했다. 전체 실행은 155.82초였다.

제품 동작은 위 범위에서 확인됐지만 자동 검수는 **FAIL**했다. 검수 코드가 취소를
`CancellationSource != ""`로 세었기 때문이다. 이 필드는 연결 취소가 전달되지 않을 때의
`native_abort_receipt` 보조 취소 경로의 근거이며 일반 연결 취소는 비어 있다. 제품은 두 경로 모두
`CANCELLED`로 판정한다. 정상 취소를 실패로 세는 검수 오류를 수정하여, 최근 16건에서
밀려난 요청도 보존하는 `Totals.Failures["CANCELLED"]`를 사용한다. 일반 전달 실패와 취소,
timeout 구분, 최근 기록에서 밀려난 뒤의 취소 집계와 native receipt 경로를 로컬 검사로
대조하여 통과했다. 제품 취소 조건과 오류 처리는 바꾸지 않았다.

실제 ledger는 **5회 생성, 9회 전송 전 거부**를 기록했다. 보조 요청에도 검증용 모델·effort·
횟수 상한을 유지했으며 거부된 요청은 backend 지출에 포함되지 않는다. 최종 최근 기록에는
보조 요청의 `UPSTREAM_FAILURE`도 남아 있으므로 모든 보조 기능의 성공을 주장하지 않는다.
전체 40회 상한을 소진하여 수정한 검수 코드의 실제 backend 재실행은 **NOT_RUN**이다.
위 화면·제품 진단 관측과 최초 assertion 실패, 수정 후 로컬 검사를 구분한다.
이번 `/compact`는 수동 실행이며 native 자동 압축 threshold 도달 검증은 아니다.

push·PR·병합·태그·Release·설치본·AdGuard 설정은 변경하지 않았다.
