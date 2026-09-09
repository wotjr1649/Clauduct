# GPT Agent 선택 계약 — 2026-09-09 확정

이 문서는 사용자가 확정한 수용 기준이며 현재 구현 완료를 뜻하지 않는다.

## 선택 의미

| 입력 | 적용 규칙 |
|---|---|
| astra / sol / terra / luna 명시 | 해당 GPT 모델과 src/models.mjs의 모델 기본 effort. 역할 기본값보다 우선 |
| inherit 명시 | 자식 생성 시점의 직접 부모가 실제 사용하는 모델과 effort 모두 상속. 역할 기본값보다 우선 |
| 모델 생략, Plan | sol/xhigh |
| 모델 생략, Explore 또는 general-purpose | luna/max |

부모 astra/max 아래에서도 inherit를 명시한 자식만 astra/max를 상속한다. 생략은 상속이 아니며 모든 후손에 최상위 모델을 강제하지 않는다. 손자는 직접 부모를 기준으로 한다.

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
