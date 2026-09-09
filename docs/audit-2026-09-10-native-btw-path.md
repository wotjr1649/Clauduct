# /btw 보조 질문 경로 정적 대조

## 기준과 판정

활성 C:/Users/JS/.local/bin/claude.exe와 C:/Users/JS/.local/share/claude/versions/2.1.266의 SHA256을 재확인했다. 둘 모두 d2c5f7b3b6a12819097ceb6efbce2a390157166003fcaee32dbde0e6d7b45ef7이다. 설치 파일의 포함 JavaScript를 데이터로 읽었으며 바이너리나 추출 코드를 실행하지 않았다. 아래 심볼과 byte offset은 이 빌드에만 해당한다.

Verified: /btw는 명령 자체가 local-jsx라는 이유로 모델 미호출이 아니다. 별도 side_question 쿼리를 실행한다. 일반 Agent 생성 호출·SubagentStart·metadata 기반의 clauduct-inherit와는 다른 경로다.

Inference: 메인의 cacheSafeParams로 실행하는 로컬 /btw는 메인 agentContext를 유지하므로 일반 메인 header/모델 경로로 gateway에 도달하는 구조다. 실제 request body·beta·effort·반환은 사용자 실행 증거가 필요하다. 이번 대조에서 런타임 수정이 필요한 실패는 확인하지 않았다.

## 연결 단계

| 단계 | 설치 코드 근거 | 의미 |
|---|---|---|
| 명령 | 191901640, name=btw, local-jsx, immediate, thinClientDispatch=control-request | UI 또는 control 요청이며 shell 명령이 아님 |
| 로컬 UI | 213448303 부근 iOe 호출 | 로컬은 cacheSafeParams로 질문 실행. remote 연결이면 별도 control-request이므로 이번 로컬 판정과 구분 |
| 질문 | iOe, 204041157 부근 | nk에 querySource/forkLabel=side_question, maxTurns=1, skipCacheWrite/skipTranscript=true, 별도 abortController 전달 |
| 도구 | iOe의 canUseTool | 도구 사용을 deny한다. nk의 noTools 옵션은 이 호출에서 지정하지 않으므로 요청 schema에 도구가 전혀 없다고 단정하지 않음 |
| 문맥 복제 | nk 189687186 → Xyn 189683029 | options와 agentContext를 원래 문맥에서 유지한다. 내부 agentId 생성과 HTTP의 agentContext.agentId는 다른 필드 |
| 모델·effort | Xor 191191089, vf 185048050, su 185047797 | 유효 mainLoopModel/permission layer와 현재 effort 선택을 공통 query로 전달. /btw에서 특정 GPT를 하드코딩하지 않음 |
| 모델 요청 | Xor의 공통 query 옵션 191212642 → query client 생성 191745430 → FU 188684554 | agentContext·model·effort가 공통 클라이언트 경로로 연결 |
| HTTP 식별 | FU와 Lh 184715827 | main agentContext면 자식 header를 생략하고 session header를 사용. 비-main이면 해당 context의 agentId/parentAgentId 사용 |
| 반환 | iOe의 결과 추출 | text 응답, synthetic 오류/도구 시도 안내, 취소 null을 구분. 사용자 UI 결과를 메인 JSONL에 자동 보존한다고 가정하지 않음 |

skipTranscript=true이므로 /btw 전용 자식 JSONL이나 metadata가 없어도 그 자체로 실패가 아니다. gateway의 agentRef=null도 메인 문맥 경로라면 예상 가능하다. 반대로 이를 일반 명시 inherit의 관계 검증을 생략할 근거로 사용하지 않는다.

## provider와 fallback 경계

FU의 firstParty 분기는 ANTHROPIC_BASE_URL을 사용하므로 Clauduct가 지정한 loopback에 연결하는 경로가 있다. Pe/Al은 별도 provider 선택을 하며 Bedrock/Vertex/Foundry/Mantle/cloud gateway 등의 분기는 독립적이다. 전체 provider 설정과 fallback 전수 검증을 이 대조로 완료 처리하지 않는다. 전역 환경·설정은 바꾸지 않았다.

Xor와 iOe에는 fallback 및 표시 처리도 있다. 이것은 Clauduct의 서버 fallback 지원 증거가 아니다. src/native-beta.mjs의 미지원 서버 fallback 등 명시 거부를 유지한다. 실패 시 beta allowlist 확대나 다른 provider로 우회하지 않는다.

## gateway 검증과 한계

src/native-gateway.mjs는 x-claude-code-agent-id 유무로 자식을 판정한다. src/native-protocol.mjs prepareNative는 메인 경로에서 요청 output_config.effort 및 허용된 system effort를 처리한다. 요청을 /btw로 분류하기 위해 질문 본문을 검사하거나 별도 모델 정책을 추가하지 않았다.

Verified: node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-protocol.mjs 통과. 실제 외부 요청·인증 조회는 0이다. 이는 일반 변환 회귀이며 /btw 실증이 아니다.

Not verified: 실제 /btw 반환·문맥 접근·effort·beta 호환성, main 작업과의 동시 실행, 자식 문맥에서의 실행, 취소 UI, remote control 및 fallback. status는 querySource를 노출하지 않으므로 요청 번호만으로 /btw라고 단정하지 않는다.

Blocked by: 기존 자동 인증 실행 제한을 유지한다. 사용자용 session-12는 정지된 메인에서 준비 상태 조회 → 사용자의 /btw 1회 → UI 결과를 첨부한 종료 상태 조회로 한정한다. skipTranscript에 대비해 사용자가 UI 답변을 제공하며, 사용자 제공 결과와 gateway 직접 증거를 구분한다. 별도 자식/Workflow를 만들거나 기존 정상 상속 시험을 반복하지 않는다.
