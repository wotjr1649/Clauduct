# 남은 실제 검증

## 현재 판정 — 원본 기록 재확인

아래 번호별 프롬프트는 과거 검증 절차다. 이미 통과한 항목을 반복 실행하라는 뜻이 아니다. 전체 완료 판정은 보류한다.

| 요구사항 | 원본 증거 | 판정 / 남은 범위 |
|---|---|---|
| compact 구간별 시간 계측 | f236f867 61행 request 11: admission 0.15ms, 첫 이벤트 2212.41ms, 첫 텍스트 10776.14ms, 첫 전달 10776.18ms, 완료 64177.91ms, 재시도 없음 | 실제 계측 확인. upstream 내부의 세부 연산 시간은 미확인 |
| 수동 compact 후 새 Read | 922e6ba0 84행 manual 경계 뒤 112/113행 models.mjs Read 성공 | 통과 |
| compact medium 적용 | f236f867 40행 manual 64806ms, 31511→9820; request 11 requested=max/actual=medium 성공 | 통과. 서로 다른 입력의 max/medium 시간을 통제 실험으로 해석하지 않음 |
| 자동 compact 및 후속 요청 | b6d81841 119행 auto 68608ms, 98097→39330; 142행 request 36 medium 성공; 137/138행 새 Read 성공 | 검증 모드 autoCompactWindow=100000에서 통과. 기본 400K와 각 자식 내부 발동은 미확인 |
| native 역할·환경 상속 | f236f867 원본 진단 Explore/general-purpose=luna/max, Plan=sol/high; 500000/500000/83.33333333333334 상속 | 당시 정책 통과. 현재 Plan=sol/xhigh의 역할 선택은 실제 재확인 필요. 환경 상속은 backend 용량 증거가 아님 |
| 현재 sol/xhigh 명시 선택·재개 | c5a3b05b request 11/12 explicit-metadata, 19/21 verified-resume; 두 Read 및 완료 | 통과. 모델 명시 없는 Plan 역할 검증으로 대체하지 않음 |
| native code-review low 완료 | 268e9bf2 자식 a91eb3a1de1548dde: Bash diff 결과 연결, 최종 지적 반환, 부모 267행 completed; 270행 request 4/5 luna/max native-fork 성공 | low 실제 완료. strict diff 호출 수용과 약 10분 31초 요청의 ping 41회 및 완료 확인. 다른 review 수준·병렬 전체 완료는 미확인 |
| native 병렬 code-review 최종 반환 | HIGH-03: 1d6ef4cd 루트/직접 자식 13개/손자 4개 대상 Read, 부모 518행 completed, 521/522행 ReportFindings 5건. request 717/719 처리 13052.10ms 겹침 | 최종 반환 통과. 자식 task-notification 복귀 CALL 한 건과 도구 오류가 남아 오류 없는 완료는 아님. [상세 감사](audit-2026-09-09-native-high-03.md) |
| peer 메시지 기반 복귀 | HIGH-03 원본 537행 request 717/730/735/740: luna/max verified-peer-resume success=true | 실제 통과. native task-notification 재기동은 별도 미지원 경로 |
| 리뷰 스킬 재호출 감소 | HIGH-03 연결된 18개 실행에서 superpowers/code-review Skill 호출 0, claude-api 3 | 이번 실행에서 확인. 일반적인 모델 준수 보장은 아님 |
| Codex 보조 metadata 이벤트 | HIGH-03 마지막 16개 진단 auxiliaryMetadataEvents=0 | 실제 발생·처리 미확인. 로컬 합성 검사만 통과 |
| 실제 인증 갱신·수시간 실행 | 사용자 지정 제외 | 이번 단계에서 실행하지 않음 |

현재 우선순위는 UNSUPPORTED_EVENT의 정확한 원인 식별이다. 과거 event=other에는 원래 이벤트 이름이 없어 복원할 수 없다. 후속 진단은 고정 구조 분류를 추가했지만 실제 원인 해결 증거는 아니다. 인증된 실제 실행은 기존 거부를 우회하지 않으며, 합성 검사 통과만으로 미확인 행을 통과 처리하지 않는다.

전제: 기존 live/인증 실행 거부 때문에 에이전트가 실제 Claude를 대신 실행하지 않는다. 사용자가 실행한 세션의 JSONL과 정제된 시간 진단을 확인한다. 전역 설정, hook trust, 모델 기본값, 권한은 변경하지 않는다. 실제 인증 갱신과 수시간 장기 실행은 이번 단계에서 제외한다.

## 1. compact 최적화 적용과 품질

현재 Clauduct를 종료하고 업데이트된 실행기로 기존 세션을 재개한다.

```powershell
D:\AIDEV\Clauduct\clauduct.cmd --model luna --effort max --resume 922e6ba0-e03b-44de-81a2-b68dcb217bf3
```

먼저 아래 짧은 사전 점검 프롬프트를 입력창에 직접 붙여넣는다. 파일을 Read로 읽게 하면 tool_result 경로가 되어 같은 검사가 아니다. 이것은 native /compact 실행이 아니라 실제 Claude 전송 경로에서 템플릿 라우팅만 확인하는 검사다.

```text
CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.

- Do NOT use Read, Bash, Grep, Glob, Edit, Write, or ANY other tool.
- You already have all the context you need in the conversation above.
- Tool calls will be REJECTED and will waste your only turn — you will fail the task.
- Your entire response must be plain text: an <analysis> block followed by a <summary> block.

This is a compact routing preflight, not a conversation summary. Do not analyze the conversation or perform work. Return exactly this fixed response:
<analysis>Routing check only.</analysis><summary>COMPACT-PREFLIGHT-OK</summary>

REMINDER: Do NOT call any tools. Respond with plain text only — an <analysis> block followed by a <summary> block. Tool calls will be rejected and you will fail the task.
```

응답 직후 다음 프롬프트로 결과를 수집한다.

```text
허용된 셸 도구로 node D:/AIDEV/Clauduct/src/request-status.mjs를 정확히 한 번 실행해.
진단 JSON을 생략하거나 고치지 말고 그대로 출력해.
직전 사전 점검 요청에서 purpose=compact-template, requestedEffort=max, effort=medium, compactShape.matches=true, success=true인지 확인해.
뒤따른 일반 진단 요청이 conversation/max인지도 확인해.
기록이 없거나 조건이 다르면 실패 또는 미확인으로 보고하고 중단해.
/compact를 실행하지 마. 파일·설정·권한을 변경하거나 환경 변수·인증 파일을 별도로 조회하지 마.
명령이 거부되거나 실패하면 우회·재시도하지 말고 오류만 보고해.
```

이 사전 점검이 실패하면 긴 compact는 반복하지 않는다. compactShape와 시간 진단부터 분석한다. 성공해도 native의 실제 compact 요청과 같은 형태임을 완전히 입증한 것은 아니므로, 다음 단계에서 manual boundary와 진단 요청 시각을 연결해야 한다.

사전 점검 통과 후에만 다음 명령을 입력한다.

```text
/compact 검증 식별자, 네 모델 기본 effort, 파일 수정 금지와 남은 검증 항목을 보존해.
```

완료 후 아래 프롬프트를 입력한다.

```text
압축 이전 검증 식별자와 파일 수정 금지 조건을 말해.
Read 도구를 새로 호출해 D:/AIDEV/Clauduct/src/models.mjs를 읽고 네 모델의 기본 effort를 표로 답해.
이어서 허용된 셸로 node D:/AIDEV/Clauduct/src/request-status.mjs를 한 번 실행하고 JSON을 그대로 보여줘.
파일·설정을 수정하거나 인증 값을 별도로 조회하지 마. 실패하면 우회하지 말고 보고해.
```

통과: compact 요청에서 purpose=compact-template, requestedEffort=max, effort=medium, 같은 모델과 성공 상태를 확인한다. 이후 일반 요청은 max로 돌아오고 기억 복원·새 Read 결과 연결이 성공해야 한다. duration과 요약 길이를 기록하되 대화 길이가 달라진 순차 실행을 엄밀한 동일 조건 벤치마크라고 부르지 않는다. 템플릿이 일치하지 않으면 목적 분류를 조정하기 전에 설치 native와 요청 형태를 다시 확인한다.

## 2. native 역할과 컨텍스트 환경 상속

같은 세션에서 아래 프롬프트를 입력한다.

```text
Clauduct의 실제 native 서브에이전트 연결을 검증해.
Agent 도구로 Explore, Plan, general-purpose를 각각 한 번씩 순차 실행해. 모델을 명시적으로 지정하지 마.
각 에이전트는 Read로 D:/AIDEV/Clauduct/src/models.mjs를 읽고 자신에게 지정된 역할의 기본 모델과 effort를 코드 근거로 보고해.
각 에이전트가 끝날 때마다 메인의 허용된 셸에서 node D:/AIDEV/Clauduct/src/request-status.mjs를 한 번 실행하고 JSON을 그대로 보여줘.
에이전트에게 셸 도구 실행을 요구하지 마. 역할과 hook 환경은 진단의 role, roleRegistered, model, effort, agentContextPolicy로 확인해.
모델의 자기소개를 실제 실행 모델의 증거로 쓰지 마.
관련 진단 값이 없으면 추측하지 말고 미확인으로 보고해. 도구가 거부되면 권한을 바꾸거나 우회하지 마.
파일이나 설정은 수정하지 마.
```

통과: native Agent 생성 및 역할 hook과 실제 요청 진단에서 Explore/general-purpose=luna/max, Plan=sol/high, roleRegistered=true를 대조한다. 일반 실행에서는 각 agentContextPolicy가 window=500000, autoCompactWindow=500000, compactPercent=83.33333333333334여야 한다. evidence=subagent-start-hook-environment는 실제 hook 프로세스 환경을 뜻하며 native 내부의 최종 context 계산이나 backend 용량까지 증명하지 않는다. 사용자 입력에 단순히 역할명이 있었다는 사실은 증거가 아니다. hook이 생략돼 값이 없으면 미검증으로 유지한다. 기존 hook 등록에 수치가 추가됐으므로 이 버전을 처음 사용하는 경우 gateway를 재시작해야 한다. 최근 16개 요청에서 앞 역할 기록이 밀려나지 않도록 에이전트별로 진단을 수집한다.

## 3. 자동 압축 실제 경로의 저비용 확인

기존 정상 세션과 구분되는 새 실행에서만 검증 옵션을 사용한다.

```powershell
D:\AIDEV\Clauduct\clauduct.cmd --model luna --effort max --verify-auto-compact
```

`/context`로 500K 모델 창을 확인하고, 진단에서 autoCompactWindow=100000을 확인한다. 이 모드는 20K 출력 예약량 조건에서 약 66.7K 자동 발동 목표다. /autocompact 값 지정으로 전역 설정을 바꾸지 않는다. 자동 압축이 기존 설정에서 비활성화돼 있으면 이를 숨기거나 덮어쓰지 않고 해당 조건을 보고한다.

```text
D:/AIDEV/Clauduct의 실제 구현을 읽기 전용으로 검토해.
Read로 src/native-protocol.mjs, src/native-gateway.mjs, src/native-transport.mjs, src/test-native.mjs, src/test-native-protocol.mjs를 순서대로 각각 한 번씩 읽어.
누락된 범위가 있으면 필요한 범위만 추가로 읽되 총 Read 호출은 10회를 넘기지 마.
라우팅, 스트리밍, 오류 처리 검증이 서로 연결되는지 근거를 들어 1000자 이내로 보고해.
/compact를 직접 실행하거나 문맥 채우기용 반복 출력·파일·루프를 만들지 마.
마지막으로 허용된 셸에서 node D:/AIDEV/Clauduct/src/request-status.mjs를 한 번 실행하고 JSON을 보여줘.
파일·설정·권한은 변경하지 마. 도구가 거부되면 우회하지 말고 보고해.
```

통과는 transcript의 compact_boundary에서 trigger=auto, 압축 후 토큰 감소, 후속 요청 성공으로 판정한다. 이 작업량으로 기준에 도달하지 않으면 자동 발동 미관찰로 기록하고 정상 개발 중 관찰을 이어간다. main의 auto compact 성공을 모든 subagent의 auto compact 성공이나 실제 400K/500K backend 수락으로 확대하지 않는다.
