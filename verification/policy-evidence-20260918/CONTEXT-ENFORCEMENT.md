# 정확 계수 기반 모델별 압축 제어

2026-09-18. 사용자가 정확 계수를 우선하고 추가 지연을 최적화하면서 적용하도록 결정한 뒤 구현했다.
이 문서는 직전 `COUNT-TOKENS.md`의 “생성 사전 검사 미적용” 상태를 갱신한다.

## 적용한 동작

- 제품 실행기의 모든 일반 생성 요청은 backend에 보내기 전에 변환된 실제 입력을 계수한다. 검증한 텍스트는 로컬, 도구·이미지·PDF·암호화 추론 등 지원 범위는 기존 구독 WebSocket `generate:false`를 사용한다.
- Astra는 입력 450000 이상, Sol·Terra·Luna는 239000 이상에서 일반 생성을 멈추고 native 압축 복구 신호를 반환한다. 압축 자체도 정확 계수하며 각 모델의 전체 입력 window 500000/272000 이상이면 거부한다. 출력 토큰 사전 상한은 기존 backend 제약 때문에 보장하지 않는다.
- 같은 native Agent와 자식 ID를 유지한다. 압축은 그 agent의 확정 모델·effort를 유지하며, 성공한 압축 뒤에도 다음 요청 전체가 목표 미만인지 다시 검사한다. 부족한 압축은 반복하거나 작업을 재실행하지 않는다.
- 이 gateway가 이미 관측한 큰 모델에서 작은 모델로 바뀌어 새 목표를 넘으면, 이전 모델·effort로 압축한 뒤 새 모델로 재계수한다. native `/model` UI 조작 자체를 지연시키는 기능이 아니다. 이 전환의 gateway 상태 검사는 통과했지만 실제 interactive picker 전체 왕복은 미검증이다.
- 확정 선택을 검증하지 못한 자식은 제품 경로에서 계수·생성 전에 거부한다. 계수 endpoint의 실패는 그 요청에 국한된다. 생성의 필수 사전 계수도 독립적으로 실패하면 그 생성은 중단한다. native 추정값으로 정책 검사를 통과시키지 않는다.
- native 공통 환경 기본값은 500000 window / 500000 auto window / 100%다. native가 표시하는 공통 500K와 gateway의 모델별 한도는 구분한다. 사용자의 명시적 환경 override는 기존 우선순위를 유지한다.
- status 최근 16건과 세션 누계를 유지한다. `countSource`, `countMs`, 정확 계수, 모델별 검사 누계·성공 계수·캐시 적중·압축 요청 수, agent별 마지막 성공 계수와 압축 대기/실패 상태를 추가했다. 실행하지 않은 모델은 `not_observed`로 남긴다.

## native 병렬 실패에서 찾은 원인과 수정

처음에는 공식 문서의 common hook fields에 따라 `PreCompact`·`PostCompact`의 `agent_id`로 연결했다.
부모가 자식을 기다리는 병렬 시험에서 `CONTEXT_COMPACTION_CONFLICT`가 발생했고 자식은 압축하지 못했다.
설치된 Claude Code 2.1.275의 read-only 코드 확인 결과, `PreCompact` 입력을 만드는 호출은 agent context를 common-field 생성기에 넘기지 않았다. hook 내부 실행기는 agentId를 별도로 받지만 command hook stdin에는 포함하지 않는다.

수정한 경로는 `PreCompact`의 stdout custom-instruction 채널에 일회성 무작위 표식을 넣는다. 실제 압축 요청이 이 표식을 되돌려주면 그 요청의 session/agent headers와 기존 확정 선택으로 연결한다. 순서나 역할 이름으로 추정하지 않는다. 표식은 backend 계수·생성 전에 제거한다. 5분 유효기간, 재사용 금지, session 검사, 명시적 agent가 있을 때 일치 검사, 최대 128개 제한을 적용했다.

`PostCompact`는 같은 식별 공백이 있으므로 완료 판정의 근거로 사용하지 않는다. 실제 압축 응답의 성공과 다음 입력의 정확 재계수를 사용한다. 압축 문구만으로는 실행 예외를 얻지 못한다. 실패·전달 오류가 있으면 성공으로 기록하지 않는다.

## 성능과 관측한 증거

성공한 계수만 완전히 동일한 변환 입력의 SHA256으로 2분/최대 128건 메모리 캐시에 저장한다. 모델·effort·도구·schema·내용이 바뀌면 다른 키다. 프롬프트·모델 출력·암호화 추론 내용은 캐시에 저장하지 않는다. 실패는 캐시하거나 자동 재시도하지 않는다. 독립 agent 사이에서 네트워크 작업 동안 전역 lock을 잡지 않는다.

실제 구독 backend의 공개 합성 입력 시험:

| 항목 | 첫 요청 | 동일 입력 두 번째 요청 |
|---|---:|---:|
| 사전 계수 | 62 | 62 |
| 생성 backend input usage | 62 | 62 |
| 계수 시간 | 1435 ms | 0 ms (정수 ms, 1 ms 미만) |
| 계수 경로 | backend-count-warmup | exact-count-cache |

합계 3 backend attempts, 2 generations. 원문·인증값은 로그에 남기지 않았다. 근거: `go-context-enforcement-live.jsonl`.
별도 실제 장문 사전 검사에서는 Luna 입력 **239042 tokens**를 **1348 ms**에 계수하고 239000 경계에서 생성 전에 거부했다. 계수 1 attempt, generation **0건**이다. 근거: `go-context-enforcement-live-boundary.jsonl`.
이것은 동일 입력 재사용 증거다. 보통 새 대화 턴은 입력이 달라지므로 여전히 backend 왕복 지연이 추가된다. 첫 토큰까지의 시간 차이 전체를 이 최적화의 효과로 주장하지 않는다. 연결 재사용·준비된 response 이어받기는 아직 구현/검증하지 않았다.

검사 근거:

- `go-context-enforcement-native-complete.jsonl`: 실제 native, 합성 count/response. 네 모델의 압축·재개, 실패 시 미재개, 한 자식만 압축할 때 격리, 두 자식 동시 압축을 각각 두 번 실행. 고유 leaf 7개 통과.
- `go-context-enforcement-unit-complete.jsonl`: 기존 회귀 포함 고유 leaf 1047개 통과. 이 실행 뒤 추가한 적대적 검사·상태 검사는 아래 별도 기록.
- `go-context-adversarial.jsonl`: 경계 -1/0/+1, 잘못된 session/agent·만료·재사용된 표식, 전체 window 초과, 실패 계수·캐시 키, 압축 실패·불충분, 작은 모델 전환의 이전 모델 유지, 미검증 자식 차단, 상태 분리. leaf 28개 통과.
- `go-context-enforcement-regression.jsonl`: 기존 native 모델·effort·ToolSearch·MCP·WebSearch·중첩 상속·계수·시작 비용 및 모듈 검사 leaf 19개 통과. 공통 표시 검사의 첫 실행은 실제 출력 `4k / 500k`의 공백을 예상하지 못해 실패했다. 값은 500K로 적용돼 있었다. 공백을 정규화해 다시 실행한 `go-context-display-docs.jsonl`에서 native 표시와 문서 검사 2개가 통과했다.
- native count/response가 합성인 검사는 계수 정확도나 요약의 의미적 품질을 증명하지 않는다. 실제 계수의 239K/450K 경계·format별 paired 검증은 `COUNT-TOKENS.md`와 그 원본 로그를 함께 참조한다.
- 첫 native 병렬 실패는 `go-context-enforcement-native-2.jsonl`에 보존했다. 후속 성공은 식별 연결을 수정한 결과이며 실패 기준을 완화하지 않았다.

## 제한과 남은 검증

- native 자체의 agent별 window 표시는 공통이다. `/context all`의 분류별 숫자는 native 자체 추정도 섞이며 gateway가 생성 전체에 적용한 계수와 같은 지표가 아니다.
- 도구 없는 요청도 정확 계수·한도를 검사한다. 다만 별도 conversation 식별 근거가 없어 그 요청의 장문 압축은 `CONTEXT_COMPACTION_UNAVAILABLE`로 거부한다. 도구 없는 대화와 제목 생성의 안전한 구분은 남아 있다.
- hook을 끄는 native 모드, 바뀐 압축 템플릿, custom instructions를 폐기하는 isolated observation agent에서는 연결을 검증하지 못하면 중단한다. 모든 native 실행 모드를 지원한다고 주장하지 않는다.
- 상태는 gateway 세션 내 최대 1024개이며 누적 한도에 도달하면 새 상태를 거부한다. 새 gateway에서 과거 전환 전 모델을 복원하는 기능과 실제 native child resume의 전체 검증은 남아 있다.
- 모든 custom-role 기본 모델·effort 발견, 결과 본문 회수/완료 판정, interactive 모델 전환 전체, 실제 장문 생성의 요약 의미 보존·처리량 검증은 완료하지 않았다.
- `-race`는 CGO 비활성·gcc 부재로 실행하지 않았다. `go vet ./...`는 통과했다.

공식 근거: [Claude Code hook reference](https://code.claude.com/docs/en/hooks), [OpenAI WebSocket mode](https://developers.openai.com/api/docs/guides/websocket-mode). 문서와 실제 2.1.275 동작 차이는 위 실패 기록과 native 코드 확인으로 구분했다.
