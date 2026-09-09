# Workflow 병렬 실제 성공과 직접 부모 상속 후속 단계

## session-09 실제 판정

992c0794-1b99-4c5f-81e3-9e15d941ab44의 메인 94행, A 자식 18행, B 자식 17행, metadata·journal·run JSON·script를 확인했다. 하나의 run wf_c4b7bb8d-43d에서 agent 2개와 parallel 1회를 사용했으며 입력·저장·run script가 일치한다.

| 근거 | 관찰 |
|---|---|
| 메인 30 / 39행 | 먼저 시험 문서 Read, workflow-authoring Skill 1회 |
| 메인 47행 | 기준점 sol/high, lifetime started/succeeded/failed=3/3/0 |
| 메인 56–57행 | call_9ryBI0bgwFTraPEBcGgIOTAC, task we6aoez4j |
| A | a61774a5b4eecc5e0, 요청 13/16 luna/high, Read 1회, A 결과만 반환 |
| B | a2cc6c5ed2b0f8ad7, 요청 14/15 sol/high, Read 1회, B 결과만 반환 |
| journal | A started → B started → B result → A result |
| 메인 71행 | 완료 알림, completed=true, 자식 2개·도구 2회·빈 결과 0개 |
| 메인 74행 | lifetime=10/10/0, 네 자식 요청 모두 workflow-result·roleRegistered=true·success=true |
| 메인 80행 | A/B 원문 결과가 최종 보고에 보존됨 |

A/B agentRef는 각각 e9db4d13a34d66e1c97626a44e3ab79b / db0de685fa763124b0b9ca49fb49f32f다. sessionRef=8fcaab776de4b4e99e82b8d6d74a559e, correlationScope=fa038c52e4e01bd0cdabedebaed1660d가 전후 일치한다. 메인은 sol/high를 유지했다. 종료 상태 이후 최종 보고 요청은 위 누계 밖이다.

요청 13/14는 startedAt이 17 ms 차이다. startedAt에서 admissionStartedMs를 빼 상대 transport 구간을 맞춘 근사 계산으로 4773.91 ms의 겹침을 확인했다. 이는 gateway 관찰 요청의 동시성이고 backend 내부 연산의 동시성은 입증하지 않는다.

Verified: 기능·gateway 요청 병렬성·기록상 절차 준수. 종료 상태 조회가 마지막 도구이며 추가 Skill은 없었다. 메인은 직접 자식 Read 기록을 보지 못해 절차를 Not verified로 남겼지만 이번 원본 조사에서 각 Read 1회와 네 키가 포함된 정상 결과를 확인했다. 전역 파일시스템의 무변경을 도구 기록만으로 보증하지는 않는다.

## 다음 단계: Agent 직접 부모 기준 손자 inherit

이 시험의 사용자 요청은 기존 --gpt-agents 정의로 부모 1개와 손자 1개를 생성하는 것이다. 메인이 sol이 아니면 clauduct-sol(sol/xhigh), 메인이 sol이면 clauduct-terra(terra/high)를 부모로 선택한다. 부모가 Agent(subagent_type=clauduct-inherit, model 생략)로 손자를 한 번 생성한다. 손자의 parentRef가 부모 agentRef와 일치하고 model/effort가 부모를 따라야 한다. 메인 모델은 바꾸지 않는다.

이 구성은 메인 모델과 직접 부모를 구별하고 기존 역할 기본 luna/max도 구별한다. 부모 정의의 기본 effort를 사용하므로 비기본 effort의 실제 다단계 상속까지 입증하는 시험은 아니다. 해당 비기본 snapshot의 로컬 통합 검사는 [직접 부모 검사](audit-2026-09-10-direct-parent-inherit.md)에 있다. Workflow 자식 상속과 Agent 정의 기반 명시 inherit를 혼동하지 않는다.

Verified: 후속 준비 중 test-agent-selection.mjs를 Node permission 제한으로 실행해 통과했다. 메인 astra/max·직접 부모 luna/high·손자 luna/high의 합성 gateway 검사와 잘못된 부모 ID의 upstream 전송 전 거부가 포함된다. 실제 Claude 실행·외부 요청은 0이다. 제품 동작 변경은 없다.

Not verified: 실제 native Agent가 해당 문맥에서 중첩 Agent 도구를 제공하고 손자 생성·Read·부모 및 메인 복귀를 완료하는지, 비기본 effort의 실제 다단계 전달. Blocked by: 기존 실제 인증 실행 및 symlink 검사 제한을 유지했다. 불가한 중첩을 Workflow·설정 변경으로 우회하지 않는다. 전체 목표 완료는 보류한다.
