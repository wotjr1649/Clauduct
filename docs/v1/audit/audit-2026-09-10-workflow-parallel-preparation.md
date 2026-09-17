# 순차 혼합 Workflow 성공과 병렬 시험 준비

## 실제 순차 시험

세션 5cd82157-c8b0-41e0-bcb4-165335d6b1f8의 메인 115행, A 자식 18행, B 자식 17행, metadata·journal·run 결과를 확인했다. 하나의 run wf_156b038f-8f7에서 A-explicit 다음 B-inherit가 실행됐다.

| 근거 | 결과 |
|---|---|
| 메인 55행 | sol/high, lifetime started/succeeded/failed=4/4/0 |
| 메인 69–70행 | Workflow 1회, call_ZKZHrBQRXYd9C09hIiRkRjpw, task wv87wszo2 |
| A | a8ee631969a82d2e8, 요청 13/15 luna/high, Read 1회 성공, A 표식만 포함 |
| B | a291c6552d2e4bb0e, 요청 18/19 sol/high, Read 1회 성공, B 표식만 포함 |
| 순서 | A 마지막 응답 15:21:26.335 UTC, B 첫 기록 15:21:26.562 UTC. journal도 A result 뒤 B started |
| 메인 84행 / 107행 | completed=true, 두 결과가 완료 알림과 최종 JSON에 보존 |
| 메인 87행 | lifetime=12/12/0, 두 자식 모두 workflow-result·roleRegistered=true·success=true, 메인 sol/high 유지 |

correlationScope=d938edae669e6bca4f6ab960fb3a80bd, sessionRef=ad750c8293caba7e548288e021f0bd83가 전후 일치한다. A agentRef=938e615cb4235fadd5b2acfb58f03022, B agentRef=8d03e9c86d495fe13828130c2481a489로 구별된다. 종료 상태 이후 요청은 12/12/0 집계 밖이다.

기능 검증은 통과했지만 메인 98행에서 superpowers:verification-before-completion을 추가 호출했다. 이는 최소 시험 절차 밖이며, 성공 선언 전에 스킬을 찾는 단계가 추가된 것으로 관찰했다. 전역 plugin/hook을 변경하지 않고 후속 시험의 Skill 허용 목록과 종료 단계를 명시한다. 실제로 이 지침을 준수하는지는 다음 기록으로 확인해야 한다.

## 병렬 로컬 검사

기존 test-workflow-selection.mjs에 동일 run의 새 자식 두 개가 합성 transport에 모두 진입해야 응답을 반환하는 검사를 추가했다. 첫 요청만 끝난 뒤 두 번째를 실행하면 2초 제한에서 실패한다. 두 자식은 서로 다른 모델/effort로 실행하며 각각의 응답·agentRef·status가 섞이지 않아야 한다. 이미 검증된 순차 검사도 유지했다. transport는 합성이고 실제 selector·loopback 등록·gateway·응답 변환·status는 제품 코드를 사용한다.

Verified: Workflow 36개, gateway 35개, native-protocol suite 통과. 동시 진입 2개와 모델별 결과 격리, 서로 다른 agentRef, 실패 누계 0을 확인했다. 외부 요청·실제 Claude 실행·인증 조회는 0이다. 제품 동작·전역 설정·hook 변경은 없다.

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-workflow-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-gateway.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-protocol.mjs
```

## 실제 후속 시험의 경계

native 작성 참조의 parallel(thunks)는 모든 작업이 끝날 때까지 기다리고 실패 슬롯을 null로 남긴다. 따라서 정확히 두 함수로 A 명시 선택과 B 기본 상속을 제출하고, null을 제거하거나 순서를 압축하지 않는다. 순차 시험과 달리 한 자식이 실패할 때 다른 자식은 이미 실행 중일 수 있다. 새 작업·재시도는 금지하고 기존 두 작업의 결과만 구분한다.

완료 조건은 두 결과의 정상 복귀, 모델/effort·agentRef 격리, 메인 설정 유지, 실제 요청 시간의 겹침이다. parallel 호출만 했거나 native가 직렬화했다면 병렬성은 미검증으로 남긴다. 겹침을 만들려고 sleep·추가 도구·반복 시험을 넣지 않는다. request 진단 시간은 gateway 관찰이며 backend 내부 연산의 동시성을 입증하지 않는다.

Not verified: 실제 native 병렬 실행·완료 복귀, 종료 Skill 미호출 준수, 중첩·resume·custom agentType·장기 안정성. Blocked by: 기존 실제 인증 실행 및 symlink 검사 제한을 유지한다. 사용자 실행 프롬프트는 docs/prompts의 session-09에 별도로 제공한다.
