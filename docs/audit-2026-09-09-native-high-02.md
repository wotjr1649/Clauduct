# Native HIGH-02 실행 감사

대상 세션: 268e9bf2-ac35-4770-bbe7-f2661b60cee1. 설치 native 기록 버전: 2.1.266. 부모 기록 667행 중 HIGH-02 준비 이후와 연결된 리뷰 루트/직접 자식 9개 JSONL 2028행을 읽기 전용으로 구조·호출·결과·오류 대조했다. 원본 메시지, 인증 값, reasoning 내용은 복사하지 않는다.

## 판정

실제 native high 본문으로 시작했고, diff 수집 및 8개 자식의 대상 Read는 성공했다. 전체 리뷰 결과 취합은 실패했다. superpowers 호출, Skill 재귀 차단, upstream 이벤트 거부, 후속 peer 메시지에 의한 재기동/라우팅 실패는 서로 다른 단계다. 단일 원인으로 합치지 않는다.

## 스킬 출처와 재귀 호출

설치된 superpowers 6.3.0의 hooks/hooks.json은 startup/clear/compact SessionStart에서 hooks/session-start를 실행한다. 해당 스크립트는 using-superpowers 본문을 additionalContext로 공급한다. 이는 설치 플러그인 기능이며 내장 code-review의 필수 종속성이 아니다. Clauduct가 기존 native 플러그인 설정을 상속하는 정책과 일치한다.

실제 리뷰 루트 6행 및 자식 7행에는 using-superpowers를 포함한 skill_listing이 있다. 목록 설명은 대화 시작 시 응답 전에 스킬을 호출하도록 요구한다. 루트는 20행에서 using-superpowers, 26행에서 claude-api를 호출했다. 루트 22행과 a88a4725197d4af30의 16행에 로드된 본문에는 특정 작업을 맡은 subagent는 이 스킬을 무시하라는 SUBAGENT-STOP 예외가 있다. 따라서 호출 후 본문을 알고도 이어지는 과도한 스킬 선택은 native가 자동으로 요구하는 정상 리뷰 절차로 볼 수 없다.

9개 실행에서 using-superpowers 8회, claude-api 6회, code-review 재귀 호출 6회가 관찰됐다. 재귀 호출은 모두 이미 forked context에서 실행 중이라는 native 오류로 거부됐다. 루트 자체에는 code-review 재호출이 없으며 문제 화면은 하위 자식에서 발생했다. 재귀 오류 뒤에도 해당 자식들은 대상 Read와 검토를 계속했다. 현재 Codex 경로의 관찰이며 native Claude 모델에서도 같은 선택을 할지는 비교하지 않았다.

## 실행·결과 연결

루트 acba4f4b481d727a9는 01:41:50.713Z 시작. 13/14행의 Bash helper와 결과가 연결됐고 untracked-added diff 11014자를 받았다. 33~40행은 동일 assistant message에 포함된 Agent 호출 8개이며 model과 subagent_type은 모두 명시되지 않았다. 다음 metadata의 parentAgentId 및 toolUseId는 이 호출들과 일치한다.

| 자식 ID | 대상 Read 결과 행 | 주요 관찰 |
|---|---:|---|
| a4011b85122a95be3 | 35 | 재귀 Skill, missing-path, 후반 IDENTITY 오류 뒤 텍스트 결과 |
| aaade69687d6d0da1 | 37 | 재귀 Skill, missing-path 2회, 텍스트 결과 |
| a60e5121d24d9cb04 | 47 | 재귀 Skill, Bash guard 거부, 후반 IO 오류 뒤 텍스트 결과 |
| af31f672f5233de44 | 38 | Bash guard 거부, missing-path 2회, 텍스트 결과 |
| a88a4725197d4af30 | 31 | 재귀 Skill, missing-path 2회, 텍스트 결과 |
| af69e4cddfa3cd90f | 22 | 재귀 Skill, missing-path, 텍스트 결과 |
| a95fd7ed70a7b97ef | 32 | 재귀 Skill, missing-path, SendMessage 28회, 텍스트 결과 |
| af4bcb265466dcf24 | 88 | Bash guard 거부, missing-path 8회, 텍스트 결과 |

전체 Read 실패 16회와 Grep의 literal glob 경로 실패 1회, Bash guard 거부 3회, Skill 재귀 거부 6회가 있었다. 이 도구 오류들은 대상 Read 8개 성공과 별개다. 하위 자식의 SendMessage는 합계 60회, Monitor 2회였다. 일부 peer 대화가 루트 실패 후에도 계속돼 재기동 오류/알림이 반복됐다. 개별 텍스트 결과의 버그 지적을 모두 확정한 것은 아니며 루트의 검증·최종 취합은 완료되지 않았다.

부모 453/475행 원본 진단의 확인 가능한 요청은 luna/max, native-fork 또는 role-default다. 447과 449의 실제 처리 구간은 약 4423.35ms 겹친다. 따라서 실제 동시 처리 증거가 있다. 진단에는 child ID가 없으므로 이 두 요청을 특정 자식 쌍에 단정 매핑하거나 8개 모두의 개별 effort를 완전 입증하지 않는다. 자식 metadata에는 명시 model이 없고 모두 general-purpose다.

## 최종 실패의 순서와 증거 한계

1. 01:56:06.786Z 시작 request 444는 native-fork, luna/max. HTTP200, ping 2회 뒤 32705.97ms에 UNSUPPORTED_EVENT / event=other로 실패했다. 최초 내용 전달은 없었고 upstream completed=false였다. other는 문자열 type이지만 response.로 시작하지 않고 진단 허용 목록에 없다는 범위만 알려준다. 원래 이름과 payload는 저장되지 않아 정확한 이벤트를 복원할 수 없다.
2. 겹쳐 진행하던 native-fork request 453은 일부 텍스트 전달 후 clientDisconnected=true, CANCELLED로 끝났다. 취소를 사용자의 수동 중단으로 단정하지 않는다.
3. 루트 253행에는 01:56:39.506Z UNSUPPORTED_REQUEST가 기록됐다. gateway 이벤트 실패와 동일 코드가 아니며 직후 별도 요청/클라이언트 처리 경로를 구분해야 한다. 근접한 request 457은 upstream 시도 없이 종료됐지만 정제 진단은 OTHER여서 정확한 요청 형태 위반 조건을 복원할 수 없다.
4. 루트 254/257/260행 등의 입력 origin은 자식의 peer 메시지다. 그 뒤 루트 256/259/262/265/268/271행에 IDENTITY 오류가 반복된다. 현재 selection은 소비된 native-fork를 새 시작으로 승인하지 않으며, verified-resume은 기존 부모의 확인된 SendMessage만 인정한다. 자식에서 루트로 오는 peer 재기동은 이 경로와 맞지 않는다. 안전한 지원에는 부모·자식 관계 및 성공한 메시지 전달 근거를 검증하는 별도 수용 기준이 필요하며 단순 identity 검사 삭제는 해결이 아니다.
5. a60e5121d24d9cb04의 IO 오류는 원래 filesystem 예외 코드가 정제돼 정확한 원인을 확인할 수 없다. 권한 거부나 파일 소실로 단정하지 않는다. finishedMs=null인 진단 행은 당시 진행 중이며 실패 수에 넣지 않는다.

과거 low의 ping 41회 성공은 유효하다. 이번 high는 이벤트 거부와 peer 복귀 호환성이라는 다른 문제를 드러냈다. max 추론 자체나 superpowers 하나가 모든 오류를 일으켰다고 결론 내릴 수 없다.

## 후속 수정 우선순위

1. 미지원 이벤트/요청/metadata IO의 비밀 없는 고정 진단을 보완해 정확한 거부 조건을 구분한다. 알 수 없는 이벤트를 일괄 무시하지 않는다.
2. 실제 peer 재기동 순서를 재현하고 기존 확인된 관계·메시지 증거로만 복귀를 승인하는 검증을 추가한다. 새 에이전트로 추정 승인하지 않는다.
3. 확인된 review 자식에 이미 실행 중인 스킬 문맥이 전달되도록 검토한다. 전역 superpowers/plugin/hook 변경이나 native 재귀 방지 해제는 하지 않는다.
4. 위 문제의 로컬 재현·회귀 검증 뒤 한 번의 high 실제 수용 검사를 진행한다. 같은 프롬프트 반복 실행은 먼저 하지 않는다.

이번 변경은 감사 문서와 검증 판정 갱신뿐이다. 실행 코드 수정·live 재실행·전역 설정 변경은 없으며, 새로운 런타임 검사를 통과했다고 주장하지 않는다.

## 후속 구현과 로컬 검증

후속 사용자 승인에 따라 다음 수정을 원인별로 적용했다. 위 문단은 최초 감사 커밋의 범위다.

- cb09130: failureStage, selectionFailure, 고정 selectionIoCode로 요청 준비/라우팅/상류/출력 검사 단계와 I/O 원인을 구분한다. 허용되지 않은 예외 코드·경로·이벤트 이름·payload는 노출하지 않는다. 모든 알 수 없는 이벤트를 허용한 변경이 아니다.
- b5f520b: 실제 to=code-review 이름 사용을 반영했다. 이름을 전체 registry에서 검색하지 않고 이미 검증된 발신자의 직계 부모 이름과 일치할 때만 실제 ID로 연결한다. 성공한 PostToolUse SendMessage, 동일 세션 관계, 변경되지 않은 대상 metadata가 필요하다. 부모 ID를 발신 자식 ID로 바꾸지 않으며 전달 증거는 한 번 소비한다. 다른 세션/발신자/metadata 변조/사용자 중단/replay 거부와 hook HTTP 경로 복귀를 검사했다. 새 source는 verified-peer-resume이다.
- 36bdc58: 검증된 리뷰 루트에서 Agent/Task 자식·손자 및 복귀에 reviewContext를 상속한다. gateway는 이 범위에만 이미 실행 중인 리뷰 본문을 직접 수행하도록 안내한다. 일반 요청/compact와 도구 목록은 유지하며 native Skill 재귀 방지나 전역 plugin/hook을 바꾸지 않았다. 실제 모델이 지침을 지키는지는 live 검사 대상이다.
- OpenAI 공개 Codex parser에서 보조 이벤트로 취급하는 codex.response.metadata를 좁은 envelope(type, metadata object, 선택적 response_id/비음수 정수 sequence_number)로 처리한다. 정상 created 이후/완료 이전 및 응답 ID 일치를 요구한다. 추가 output/error 필드나 malformed envelope는 거부한다. payload를 답변·진단에 옮기지 않으며 실제 발생 건수만 auxiliaryMetadataEvents에 남긴다. 정상 응답 동일성, 변조·순서 오류 거부, gateway 완료 및 payload 비노출을 검사했다. 근거: https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/sse/responses.rs 의 process_responses_event. 공개 main 확인일 2026-09-09. 과거 event=other가 이 이벤트였다는 직접 증거는 없으므로 과거 장애 원인이 완전히 해결됐다고 단정하지 않는다.

로컬 검증: agent-selection(직접 검사와 loopback hook 복귀), file-review, native-protocol, native-gateway 21/21 및 native 45/45. 실제 인증을 사용한 실행은 수행하지 않았다. 마지막 16개 요청만 보존하는 기존 진단 범위, 확인된 registry 최대 1024개/대기 증거 5분 유지 범위는 동일하다. 임의 sibling 이름이나 관계를 입증할 수 없는 peer 복귀는 계속 거부한다. 실제 high 전체 완료·스킬 재호출 감소·이전 미지원 이벤트/요청의 정확한 원인은 아직 미검증이다.
