# Workflow 명시 선택 성공과 혼합 자식 시험 준비

## 실제 명시 선택 성공

660a5d7d-b643-47e2-b319-a2ef68a97121의 메인 97행, 자식 18행, metadata·journal·run JSON·script를 확인했다. 927eefdf의 IDENTITY 수정 이후 단일 명시 선택은 실제 통과했다.

| 근거 | 관찰 |
|---|---|
| 메인 56행 | sol/high, lifetime started/succeeded/failed=4/4/0 |
| 메인 64–65행 | call_7mVhxRb7Zbpnz9hyBwhPtrZq, task wvzuxjfh5, run wf_f2c24d2e-cfc |
| Workflow script | 입력·저장·run JSON의 script 일치. agent 1회, gpt-5.6-luna/high 명시 |
| metadata | workflow-subagent, model=gpt-5.6-luna |
| 자식 13–14행 | models.mjs Read 1회, 네 키 포함, 오류 없음 |
| 자식 17–18행 | reasoning 다음 최종 text. 네 키와 WORKFLOW-EXPLICIT-COMPLETED 포함 |
| journal / run | 최종 text와 같은 48자 문자열, completed=true, 도구 1회·자식 1개 |
| 메인 75행 / 85행 | 완료 알림 뒤 동일 결과가 최종 보고에 보존됨 |
| 메인 82행 | 자식 요청 12/14 luna/high, workflow-result, roleRegistered=true, success=true. 메인은 sol/high 유지. lifetime=9/9/0 |

두 진단의 correlationScope=aeffe8c91600c03138528e457933fe1a, sessionRef=04569b62951b4fc19c6a08bd06b8da2b가 일치한다. 자식 agentRef=80d4d15a5daa351fb31ac4ae18089549다. luna 기본 max가 아닌 명시 high가 적용되었으며 부모 sol과도 구별된다. 마지막 상태 수집 이후 요청은 위 누계 밖이다. 다른 세션의 모델을 이 값으로 가정하지 않는다.

## 다음 단계와 로컬 검증

이번 실제 시험은 같은 Workflow run에서 A 명시 선택 → 성공 확인 → B 기본 상속의 순차 자식 2개다. A 실패 시 B를 생성하지 않는다. 병렬, 중첩, resume, custom agentType은 추가하지 않는다. A/B의 서로 다른 완료 표식과 agentRef별 실제 모델을 함께 비교한다.

기존 loopback 시험을 보강했다. 동일 run에서 첫 자식의 명시 luna/high 또는 terra/xhigh를 검증한 뒤 journal에 첫 자식 result와 두 번째 자식 started를 함께 둔다. 새 자식은 부모 sol/medium을 사용하고, 기존 등록 자식의 후속 요청은 자신의 명시 모델을 유지하는지 검사한다. 응답에는 합성 모델별 식별 문자열을 사용하여 응답 교차도 검출한다. 기존 등록의 후속 요청 검사는 Stop 이후 resume 지원을 의미하지 않는다.

Verified: Workflow 34개, gateway 35개 및 native-protocol suite 통과. run 연결 1회로 두 ID가 처리되고 agentRef는 다르며 기존 ID의 agentRef는 유지된다. 전송 model/effort, 개별 결과, 실패 누계 0을 검사했다. 외부 요청·실제 Claude 실행·인증 조회는 0이다.

```powershell
node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-workflow-selection.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-gateway.mjs
node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-native-protocol.mjs
```

제품 동작 변경은 없다. Not verified: 실제 같은 run의 혼합 자식 2개 실행과 메인 복귀, 병렬·중첩·resume·장기 안정성. Blocked by: 기존 실제 인증 실행과 symlink 검사 제한은 유지했다. 사용자 운영의 읽기 전용 실제 시험으로만 남은 증거를 확보한다.
