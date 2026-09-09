# GPT Agent 선택 계약 — 2026-09-09 확정

이 문서는 사용자가 확정한 수용 기준이며 현재 구현 완료를 뜻하지 않는다.

최신 상태: 정의 기반 inherit와 역할 고정값은 2f96f1df에서 실제 성공했다. 일반 개발용 호출 방식은 승인 후 --gpt-agents로 구현·로컬 검증했고 실제 수행은 남아 있다. 앞선 조사·시험 기록은 당시 상태를 보존한다.

## 선택 의미

| 입력 | 적용 규칙 |
|---|---|
| astra / sol / terra / luna 명시 | 해당 GPT 모델과 src/models.mjs의 모델 기본 effort. 역할 기본값보다 우선 |
| inherit 명시 | 자식 생성 시점의 직접 부모가 실제 사용하는 모델과 effort 모두 상속. 역할 기본값보다 우선 |
| 모델 생략, Plan | sol/xhigh |
| 모델 생략, Explore 또는 general-purpose | luna/max |

메인 모델·effort는 세션마다 달라질 수 있으며 상속 기대값은 해당 생성 호출의 실제 부모 값이다. astra/max는 과거 검증 예시이지 고정값이 아니다. 부모가 luna/max이면 inherit 자식의 기대값도 luna/max다. 생략은 상속이 아니며 모든 후손에 최상위 모델을 강제하지 않는다. 손자는 직접 부모를 기준으로 한다.

## 범위와 수용 조건

GPT 직접 선택과 명시 inherit의 실제 native 실행이 이번 완료에 필수다. Claude 모델/별칭을 대신 선택하는 테스트로 대체하지 않는다. Claude 모델 지원, GPT/Claude 전체 모델 목록 확장은 추후 과제로 이번에 다루지 않는다. 기존 별칭 구현의 삭제 여부까지 결정한 것은 아니다.

Agent 스키마, native 입력 처리, 자식 metadata, gateway 선택과 실제 반환이 일치해야 한다. 모델에 보이는 enum만 확대하고 native 실행이 지원된다고 주장하지 않는다. 검증/권한 통제를 약화하거나 거부를 우회하지 않는다.

검증에는 직접 GPT 선택, 부모의 비기본 effort 상속, 역할 기본값 보존, 직접 부모 기준 손자 상속, 잘못된 입력/관계의 거부를 포함한다. 로컬 회귀와 사용자 실행 세션 증거를 구분하고 관련 변경만 커밋한다.

## 현재 상태

Not verified: 위 계약의 전체 구현과 실제 native 실행. 기존 내부 inherit는 native 요청 모델 유지 분기이며, 부모 모델과 effort의 생성 시점 snapshot을 보장한다는 증거가 아니다.

다음 작업은 native 실행까지 전달 가능한 지원된 연결 지점 조사다. 연결 지점이 확인되지 않으면 정확한 경계를 차단/미확인으로 남긴다.

## 연결 지점 조사 — 계약 커밋 e412f14 이후

Verified: 프로젝트 소스와 설치 native 2.1.266의 포함된 JavaScript를 읽기 전용으로 대조했다. 실행 파일은 실행하거나 수정하지 않았다. 대상은 C:/Users/JS/.local/share/claude/versions/2.1.266, 218971808 bytes, SHA256 d2c5f7b3b6a12819097ceb6efbce2a390157166003fcaee32dbde0e6d7b45ef7이다. 아래 native 심볼은 이 빌드의 내부 이름이며 안정된 API가 아니다.

| 단계 | 확인한 연결 지점 | 판정 |
|---|---|---|
| 시작 설정 | src/clauduct.mjs interactiveLaunch의 --model/--effort, modelPicker, gateway /v1/models | 메인 선택 경로다. Agent model enum을 바꾸는 경로로 확인되지 않음 |
| GPT에 보이는 도구 | src/native-protocol.mjs prepareNative가 tool.input_schema를 function parameters로 전달 | 현재 그대로 전달. 여기만 변경해도 native 입력 검증은 남음 |
| native Agent 입력 | o6o의 model 고정 enum, q_n을 반환하는 Agent inputSchema | astra/sol/terra/luna/inherit 직접 인수는 없음. 강제 모델 설정 분기는 model 필드를 숨기며 확장하지 않음 |
| native 사용자 정의 agent | Lnn의 model은 비어 있지 않은 string이며 inherit 정규화, effort 필드도 존재. --agents JSON 옵션 등록 | 정의 수준의 GPT 모델/inherit 진입 후보. Agent 호출의 model 인수와 다른 계약이므로 직접 호출 요구 충족을 단정하지 않음 |
| native 실행 직전 | Agent.call의 MBo에 model, parentModel, resolveModel 전달. 결과의 agent/spawn.model을 받아 실행 | 모델 해석 연결부 존재. 내부 함수 직접 호출·후킹을 지원 API로 간주하지 않음 |
| 자식 등록 | src/agent-route.mjs bindingFrom → /clauduct/agents → native-gateway selection.resolve | 기존 hook은 역할/관계/환경 등록용. 모델·effort 선택을 반환하거나 변경하지 않음 |
| 선택 증거 | agent-selection remember의 call.selection과 metadata.model 일치 검사 | 정의 기반 선택이나 다른 표현을 추가할 때 이 신뢰 경계를 보존해야 함. 불일치 검사를 제거하지 않음 |
| 실제 GPT 선택 | native-gateway가 selection.route를 prepareNative에 전달 | 검증된 모델·effort snapshot을 적용할 수 있는 기존 연결 지점 |

inherit의 구체적 공백: agent-selection은 inherit에 route=undefined를 반환한다. prepareNative는 subagent에서 doc.output_config.effort를 사용하지 않고 모델 기본 effort를 선택하며, system output_config도 subagent에는 적용하지 않는다. native가 astra/max를 전송해도 현재 이 분기만으로 부모 max 상속을 보장하지 못한다. gateway의 remember 호출은 prepared.selected를 전달하지 않으므로 생성 호출에 부모의 실제 모델·effort를 고정하는 별도 연결이 필요하다. 이 변경은 아직 구현하지 않았다.

안전 경계: native PreToolUse updatedInput도 도구 inputSchema.safeParse 검사를 받는 코드가 있다. hook으로 비허용 model을 주입하는 것은 해결책으로 삼지 않는다. 현재 Clauduct는 --agents를 blockedOptions에 포함한다. 사용자 정의 agent 후보를 시험하기 위해 이 차단을 제거하거나 다른 경로로 같은 설정을 주입하지 않았다. 새 설정/지침/플러그인 연동을 채택하려면 그 정확한 효과와 범위를 별도로 검토해야 한다.

Not verified: 현재 native Agent(model=GPT/inherit) 그대로의 지원된 확장 API, 사용자 정의 agent 후보의 실제 실행·metadata 표현·기존 역할 보존, 부모 생성 시점 snapshot과 재개/병렬/손자 동작. 합성 테스트나 실제 계정 호출은 이번 문서 조사에서 실행하지 않았다. 연결 지점의 존재와 계약 전체의 실현 가능성을 구분한다.

결론: gateway 쪽 모델·effort 적용 지점은 확인했다. 가장 앞의 native Agent 입력 계약은 아직 해결되지 않았다. 사용자 정의 agent는 native가 제공하는 별도 진입 후보지만 현재 launcher 차단 및 호출 형식 차이가 있어 채택하지 않았다. 다음 설계는 이 후보가 기존 역할과 명시 선택/생략의 구분을 보존하는지, 필요한 설정 변경 범위가 무엇인지부터 확인해야 한다. enum 패치나 Claude 별칭 대체는 하지 않는다.

## 후속 구현 — 부모 snapshot, 2026-09-09

위 조사의 inherit 공백 중 gateway 내부 처리를 수정했다. native-gateway는 생성 호출을 기록할 때 prepared.selected를 전달한다. remember는 명시 inherit에만 모델·effort를 검증하여 불변 복사본을 기존 pending 기록에 보관한다. 관계와 metadata 선택값의 기존 일치 검사를 통과한 자식만 이 route를 사용한다. 부모 snapshot이 없거나 잘못되면 기본값으로 조용히 대체하지 않고 거부한다. SendMessage 재개는 최초 상속 route를 유지하며, 완료 알림 재개도 기존 selection 재사용 경로를 유지한다. 역할 기본값과 명시 GPT 모델 기본 effort는 바꾸지 않았다. 새 누적 Map이나 의존성은 없다.

Verified: test-agent-selection.mjs에서 수정 전 route=undefined 실패를 관찰한 후 수정 버전 통과. 생성 후 부모 객체 변경에 대한 고정, 상속 route 불변성, 직접 부모 기준 snapshot, 잘못된 부모·다른 세션 거부, 네 GPT 모델 명시/Plan 기본값/명시 inherit 우선순위, SendMessage 재개를 검사했다. loopback gateway 통합 검사에서 astra/max 부모 생성 응답 → remember → 등록·metadata 검증 → 자식 병렬 요청 두 개의 astra/max upstream 요청 및 status를 확인했다. test-completion-selection.mjs 46개, test-native-gateway.mjs 24개 통과. Node 권한을 프로젝트 읽기와 필요한 src fixture 쓰기로 제한했고 실제 Claude/외부 요청은 0이다. symlink 검사는 기존 제한으로 미실행이다.

Not verified: native Agent가 model=inherit/GPT를 수락하는 실제 실행. 추가 gateway 검사의 schema는 계층 내부 계약용 합성이며 설치 native 스키마가 아니다. 따라서 내부 상속 수정 완료와 전체 native 연결 완료는 다르다.

[공식 subagents 문서](https://code.claude.com/docs/en/sub-agents)를 2026-09-09 확인했다. 사용자 정의 agent는 전체 모델 ID/inherit와 effort를 설정할 수 있고 --agents 정의는 세션에 한정된다. 이는 설치 바이너리의 정의 parser와 일치하지만 Agent 호출 model enum 확대를 뜻하지 않는다. 현재 차단된 --agents 전달이나 agent 지침/설정 등록을 실제 사용하려면 그 정확한 범위의 권한을 먼저 확정해야 한다. 이번 변경은 그 차단·등록·전역 설정을 건드리지 않는다.

## 승인된 시험 등록

사용자가 전역 설정과 임의 --agents 차단을 유지하는 자식 세션 한정 읽기 전용 시험용 agent 등록을 허용했다. 이에 따라 --verify-agent-models 옵션에서만 실행기가 고정한 시험 정의 5개를 --agents JSON으로 전달한다. 이는 사용자 입력 --agents를 허용하는 변경이 아니다. 설치 native 바이너리, hook, 권한 정책, 기존 settings·환경은 변경하지 않는다. 일반 역할 대신 시험 전용 이름을 사용하며 실제 호출은 사용자 실행으로만 검증한다. [사용법과 한계](native.md).

Verified: test-launcher-native.mjs 통과. 일반 실행에서 미등록, 검증 옵션에서 정확히 5개 정의와 Read-only 도구 목록/maxTurns/모델/effort, 기존 settings·환경·메인 effort 보존, source 객체 불변, 임의 --agents·중복/값 첨부 옵션 거부를 검사했다. --verify-agent-models --model astra --effort max --dry-run에서 정의 이름 5개와 childStarted=false, credentialReads=0, globalWrites=0을 확인했다.

Not verified: native의 실제 정의 로딩, 실제 도구 제한, 정의 모델의 요청·metadata 표현, 정의 기반 inherit의 effort 유지. 이 등록 변경은 gateway 선택 판정을 보정하지 않는다. 따라서 시험 성공을 만들기 위한 우회가 아니라 실제 입력·출력 관찰 준비이며, 전체 계약 완료는 여전히 보류다.

## 시험 세션 확인 — 34f1e15c

사용자가 제공한 46f4a80e129350648d7d8ca740bc0cec는 status의 sessionRef이며 native 세션 UUID는 34f1e15c-dad7-49ba-b0bc-8d23982190a3이다. 원본 27행의 BEFORE request 4는 메인 gpt-5.6-luna/max, success=true다. 35행에서 model 인수를 생략한 clauduct-probe-astra 호출을 한 번 시도했으나, 36행에서 agent type not found로 거부됐다. 자식 기록은 생성되지 않았다. 표시된 사용 가능 목록에 시험 agent가 없으므로 모델 라우팅이 아니라 등록/로딩 단계에서 중단됐다. 실제 실행 명령이 없으므로 검증 옵션 누락과 다른 등록 실패 원인을 단정하지 않는다.

이 세션의 inherit 목표는 부모의 실제 luna/max이며 astra/max로 판정하지 않는다. 네 모델과 다섯 effort 조합의 내부 상속 회귀 검사를 추가했다. 이 검사는 backend의 모든 조합 수락 또는 native 정의 기반 상속 성공을 의미하지 않는다. 다음 실제 검증은 사용자가 원하는 메인 모델·effort를 유지한 실행 명령에 --verify-agent-models를 추가하고 시험 agent 로딩부터 확인한다.

## 정의 기반 inherit 연결 수정

8884c607-6659-4165-b84d-0e6040c9cedd(sessionRef 33df4ff15620086a8bcf0791ff11ccbe)에서 시험 정의 5개가 실제 생성됐고 각 자식은 Read 1회 후 완료 문구를 반환했다. astra request 10/11=medium, sol 18/19=xhigh, terra 26/27=high, luna 33/35=max였다. 최초 inherit 42/43=luna/max는 당시 부모 luna/max와 같지만 luna 기본값도 max여서 상속 입증으로 충분하지 않았다. 이후 부모 51/52 및 63/66=luna/high인데 inherit 자식 54/55 및 65/67=luna/max로 불일치했다. Agent 호출과 연결 metadata 모두 model을 생략했으며 source=role-default였다. 이 관찰을 로컬 실패로 재현했다.

수정: 실행기는 native에 전달할 시험 정의를 선택기에도 제공한다. 선택기는 생성 시 등록 정의 중 model=inherit인 유형을 고정해 저장한다. 그 유형을 선택한 Agent/Task 호출이 model을 생략한 경우에만 부모의 실제 모델·effort snapshot을 보관한다. 기존 세션·호출 ID·역할·부모·metadata.model 일치 검사 후 route를 적용하며 source=definition-inherit로 구분한다. 명시 model 인수가 있으면 기존 명시 선택이 우선한다. SendMessage 재개는 원래 상속 route를 유지하고 완료 알림도 기존 selection을 유지한다. 이름만으로 판정하지 않고 요청 본문에서 정의를 받지도 않는다. 일반 실행에는 정의를 전달하지 않으며 기존 역할 기본값은 그대로다.

Verified: 수정 전 등록 정의 경로의 route=undefined 실패를 확인한 뒤 test-agent-selection.mjs 통과. 실제 실행기가 생성한 정의 JSON을 사용해 등록/미등록 구분, 생성 후 정의 객체 변조 비적용, 부모 snapshot 누락 거부, 잘못된 호출/역할/부모/model 거부, 20개 모델·effort 조합, 재개 유지와 명시 선택 우선순위를 검사했다. native model enum을 유지하고 model을 생략한 loopback 통합 검사에서 부모 luna/high → 정의 inherit=luna/high, 일반 역할=luna/max, Plan=sol/xhigh를 각각 두 병렬 요청과 status로 확인했다. test-completion-selection.mjs 46개, test-native-gateway.mjs 24개, test-launcher-native.mjs도 통과했다. 실제 Claude/외부 요청은 0이다.

Not verified: 수정 버전의 실제 native 반환과 일반 개발용 GPT 직접 선택 인터페이스. symlink 검사는 기존 제한으로 미실행이다. 다음 실제 검증은 부모의 비기본 effort에서 inherit와 일반 역할/Plan만 확인한다. 이미 성공한 네 시험 모델은 반복하지 않는다. 그 증거를 받은 뒤 일반 호출 방식의 제공 범위를 정리한다. 시험 전용 경로의 성공을 전체 계약 완료로 바꾸지 않는다.

## 실제 상속·역할 보존 성공 — 2f96f1df

세션 2f96f1df-088c-46cc-ac9b-39f77fe09a2a는 최초 terra/high가 모델 기본 effort와 같아 시험 전에 중단했다. 이후 실행 구간의 원본 64행 BEFORE request 3과 후속 부모 요청은 terra/max였다. 87행 status의 자식 request 9/10은 terra/max, definition-inherit, success=true다. 106행의 일반 역할 request 17/18은 luna/max, role-default, success=true이며 125행의 Plan request 25/26은 sol/xhigh, role-default, success=true다. 마지막 누적 요청/성공은 각각 17, 실패는 0이다.

71/94/114행 Agent 호출은 각각 clauduct-probe-inherit/general-purpose/Plan이며 model을 생략했다. 자식 aa520561ba7cf3dfd/af6c19632c638503c/a3fe9b33de53c82c4의 metadata.toolUseId와 agentType을 대조했고, 각 자식은 src/models.mjs Read 1회, 추가 도구 0회, 도구 오류 0회, 완료 문구 반환을 확인했다. 단순 부모/기본 effort 일치가 아니라 terra 기본 high와 다른 max를 상속했으므로 비기본 effort 유지가 실제 입증됐다.

Verified: 등록된 시험 정의의 모델·effort 상속과 기존 일반 역할·Plan 고정값 보존. Not verified: 일반 개발용 명시 선택 인터페이스, 이 세션에서 시험하지 않은 손자/재개 조합, 별도 보안 검증의 잔여 항목. 전체 목표 완료가 아니다.

## 일반 호출 방식 조사

설치 2.1.266 정의 parser는 자체 prompt를 요구하며, 확인한 스키마에는 기존 내장 agent를 extends하는 필드가 없다. [공식 문서](https://code.claude.com/docs/en/sub-agents)도 사용자 정의 agent의 자체 시스템 프롬프트와 도구 설정을 설명한다. 따라서 모델별 이름만 붙여 생성한 일반 agent를 native Plan/Explore의 동일한 구현으로 간주하지 않는다.

검토안(미승인·미구현): 기존 내장 역할은 변경하지 않고, 별도 명시 옵션에서 세션 한정 일반 작업용 clauduct-astra/sol/terra/luna/inherit 정의를 제공한다. Agent의 model 인수 대신 해당 subagent_type 선택으로 GPT 모델을 선택한다. 모델 정책·관계 검증·부모 snapshot은 재사용한다. 이는 Plan/Explore의 model 인수 확장과는 다른 인터페이스이며 그 요구를 자동 충족한 것으로 처리하지 않는다. 일반 작업 도구를 제공하는 등록은 현재 Read 전용 시험 권한을 넘으므로 정확한 범위를 사용자와 확정하기 전 적용하지 않는다. 전역 설정·임의 --agents 차단은 유지한다.

## 승인된 일반 작업용 인터페이스 구현

사용자가 위 별도 활성화 옵션과 일반 개발 도구 제공을 승인했다. --gpt-agents가 세션 한정 일반 작업용 정의 5개를 생성하며, 사용법·도구 목록·권한 범위는 [native 경로 문서](native.md)의 일반 작업 agent 절에 정리했다. 기존 내장 역할이나 Read 전용 시험 정의를 교체하지 않는다.

선택기는 시작 시 등록 정의의 명시 모델·effort를 검증·불변 복사하고, 그 정의를 선택한 Agent/Task 생성 호출에 연결한다. 기존 metadata.model과 원본 호출 선택값의 일치 검사는 유지한다. 정의 기반 직접 선택은 definition-model, 부모 snapshot 상속은 definition-inherit로 출력한다. SendMessage 재개와 완료 복귀는 원래 선택을 보존한다. 새 라이브러리, plugin/MCP/hook/permissionMode 또는 전역 설정은 추가하지 않았다.

Verified: test-launcher-native.mjs와 test-agent-selection.mjs 통과. 실제 실행 인자 JSON으로 일반 5개/시험 포함 10개 등록, 기존 설정·환경·메인 선택 불변, 중복/값 첨부 옵션 및 임의 --agents 거부, 정확한 도구 필드, 네 모델별 정의 선택, 일반 inherit·손자 상속·재개를 검사했다. native enum을 유지한 loopback 요청에서 네 일반 모델의 definition-model 및 비기본 effort의 definition-inherit, 기존 일반 역할/Plan 고정값을 각 두 병렬 요청과 status로 확인했다. completion 46개, gateway 24개, native-protocol도 통과했다. 일반/시험 동시 dry-run은 main terra/max 유지, 이름 10개, credentialReads=0, childStarted=false, globalWrites=0이었다.

Not verified: 새 일반용 이름의 실제 native 로딩·작업·쓰기 도구 및 다단계 위임 수행. symlink 검사는 기존 제한으로 미실행이다. 등록 스키마/로컬 프로토콜 검증을 실제 native 도구 실행 증거로 대신하지 않는다. 이전 시험 정의의 실제 성공은 보존하되 새 일반 인터페이스 전체 완료로 확대하지 않는다.
