# away_summary 생성·모델·기록 경로

## 버전 경계

현재 C:/Users/JS/.local/bin/claude.exe는 FileVersion/ProductVersion 2.1.267.0, 크기 220051616 bytes다. C:/Users/JS/.local/share/claude/versions/2.1.267과 SHA256 23dde2a47cf1d7d9c4a2d96d21fa80ea9bfc872dfde0ee06e9982d2908603350이 일치한다. d9752fc6 세션의 version 필드도 2.1.267이다. 업데이트를 누가/어떻게 수행했는지는 조사하지 않았다.

기존 2.1.266 파일은 이전 해시 d2c5f7b3b6a12819097ceb6efbce2a390157166003fcaee32dbde0e6d7b45ef7을 유지한다. 2.1.266 기준 전체 경로 조사와 현재 활성 버전을 혼동하지 않는다. 설치 파일은 읽기만 했으며 업데이트·다운그레이드·실행·설정 변경은 하지 않았다.

## 실제 기록

d9752fc6-4099-40fb-8504-34486f086b1e.jsonl 61행에 type=system, subtype=away_summary, content 172자, timestamp=2026-09-10T04:10:09.100Z가 있다. native가 초기 요약에 붙이는 안내 suffix도 일치한다. 본문은 불필요하게 복사하지 않는다.

종료 status에 요청 13은 04:09:59.878Z 시작, finishedMs=8828.51, gpt-5.6-sol/high, success=true, clientDisconnected=false로 기록됐다. 계산상 요청 완료 약 393.49ms 뒤 요약이 기록됐다. 같은 시각 구간의 성공 요청과 결과가 대응하므로 요청 13이 생성 요청이라는 강한 추론이 가능하다. 그러나 summary에는 gateway request ID가 없고 status에는 querySource가 없으므로 일대일 연결은 미확정이다.

Verified: native away_summary 결과 기록 존재, 직전 성공 요청의 실제 sol/high, session-12 전체 관찰 요청의 동일 모델·effort. Inference: 해당 성공 요청 13이 요약 생성 요청. Not verified: 본문/응답 ID로 직접 연결한 모델 요청의 귀속. 취소 16/20은 각각 04:19:38Z/04:21:04Z 시작으로 이 요약 생성 뒤이며, 이번 요약 실패 증거로 사용하지 않는다.

## 2.1.267 정적 경로

아래 byte offset과 내부 심볼은 해당 해시의 포함 JavaScript 기준이다. 코드를 추출 실행하거나 hook/설정을 변경하지 않았다.

| 단계 | 근거 | 동작 |
|---|---|---|
| 로컬 자동 발생 | yJe, 208968100 부근 | 활성화·idle/blur 상태, cache age, rate-limit 상태, 입력 초안·background 작업·loop wakeup·최근 요약 여부 등을 검사. focus 복귀 시 진행 중 요약을 취소하는 경로 존재 |
| 생성 | b4e, 196148100 부근 | XD로 CacheSafeParams를 얻고 없으면 no-turn. 도구 deny, querySource/forkLabel=away_summary, maxTurns=1, skipCacheWrite/skipTranscript=true로 hk 호출 |
| 저장 문맥 | XD, 192055514 | 현재 root/session과 맞는 문맥만 사용. 저장 시점과 현재 세션 모델이 다르면 mainLoopModel을 현재 값으로 조정. 특정 GPT 고정값 없음 |
| 복제 | hk 192063192 → kSn 192059035 | options·permissionLayers·agentContext를 상속해 공통 uF query 실행. noTools는 이 호출에서 지정하지 않으므로 전송 schema의 도구 부재까지 주장하지 않음 |
| 모델·effort | 공통 query 옵션 192331690 | effortValue=uu(context), turnEffort=ed(permissionLayers), agentContext를 전달. 생성 시 설정을 일괄 하드코딩하지 않음 |
| client/header | 192866353 → oB 189688795 부근, zh 185784522 | model/querySource/agentContext를 전달. main context이면 session header만 쓰고 자식 header는 생략 |
| 결과 기록 | yJe 208970164 → knr 193499758 부근 | aborted/non-ok이면 기록하지 않음. 성공 text에 초기 안내를 붙일 수 있고 system/away_summary로 transcript에 추가 |

생성기는 40단어 미만의 짧은 요약을 요청하고 추출 text를 400자 한도로 자른다. UI 안내 suffix는 그 뒤 붙을 수 있다. API 오류는 별도 kind로 반환되며 자동 기록부가 정상 요약으로 저장하지 않는다. skipTranscript는 생성 fork의 독립 transcript를 생략한다는 뜻이지 최종 system/away_summary 기록이 없다는 뜻이 아니다.

2.1.266의 KVe → nk/Xyn → sZn 경로와 핵심 구조는 대응한다. 2.1.267에는 hk의 skipMessageCacheMarkers와 공통 query의 turnEffort 등 필드 차이가 있으므로 단순 심볼 변경만 있었다고 주장하지 않는다. 이 조사에서는 away_summary와 side_question의 해당 경로만 대조했다.

## 결론과 한계

away_summary는 별도 GPT 모델을 선택하는 일반 Agent가 아니라 저장된 메인 문맥을 사용하는 보조 쿼리다. 결과가 실제 생성·기록됐으며 관찰된 gateway 요청은 sol/high다. 정확한 request 귀속은 시간상 추론으로 남긴다. 코드 수정이 필요한 실패는 발견하지 않았다. 불확실성을 없애기 위해 원본 프롬프트/응답을 추가 로깅하거나 전역 설정을 바꾸지 않는다.

Not verified: 자동 발생 조건 전 조합, 모델 변경 중 생성, focus/취소 경쟁, remote recap, provider/fallback 전수 호환성, 2.1.267 전체 Agent/Workflow 스키마·hook·출력 계약. 기존 자동 인증 실행과 symlink 제한을 유지한다. 문서만 변경했으므로 런타임 회귀를 새로 실행했다고 주장하지 않는다.

다음 우선순위는 추가 사용자 시험이 아니라 2.1.267의 Agent/Workflow 연결 계약을 기존 2.1.266 근거와 읽기 전용 대조하는 것이다. 확인된 차이에 한해서 필요한 로컬 검사나 사용자 시험을 정한다.
