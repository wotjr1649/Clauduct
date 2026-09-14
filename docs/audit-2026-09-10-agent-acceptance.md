# GPT 선택·상속 수용 조건 대조

핵심 선택·상속 계약의 기능 증거가 확보됐다. 제품 전체 완료 또는 모든 native 경로의 호환성 완료는 아니다. 이번 작업은 원본 재확인, 현행 로컬 검사, 문서 정리뿐이며 런타임·설정 변경이나 실제 모델 재호출은 없다.

## session-11 실제 증거

대상 a2d50ff0-7070-491e-959b-ad5bba26d672. C:/Users/JS/.claude/projects/D--AIDEV-Clauduct-verification-dev-sandbox-run-01/ 아래 메인 JSONL 94행, 부모 agent-abfd538362b8c815a 22행, 손자 agent-a0fcd59757ce6ea2a 18행 및 metadata를 읽었다.

| 대상 | 요청 | 실제 model/effort | 선택 근거 |
|---|---|---|---|
| 메인 | 기준점 및 종료 상태 | gpt-5.6-sol/high | 메인 값 유지 |
| 부모 | 12/13 | gpt-5.6-sol/high | definition-inherit |
| 손자 | 15/17 | gpt-5.6-sol/high | definition-inherit |
| 부모 복귀 | 20 | gpt-5.6-sol/high | verified-completion-resume |

모두 success=true다. MODELS.sol의 기본 effort는 xhigh이므로 high는 비기본 값이다. 부모 agentRef 8185c10756ae2c9b289d13451e236b09와 손자 parentRef가 일치한다. 손자 agentRef는 a3c48cac1b532f313541c23c16cdb071이다. metadata에서도 parentAgentId=abfd538362b8c815a, spawnDepth=2이며 부모 깊이는 1이다. 두 Agent 호출 모두 model 인수를 생략했고 metadata.toolUseId와 일치한다.

손자 Read 1회 후 네 모델 키와 NONDEFAULT-GRANDCHILD-COMPLETED를 반환했다. 부모는 그 원문을 보존하고 NONDEFAULT-PARENT-COMPLETED를 추가했다. 부모 19행 완료 알림 → 21행 반환, 메인 72행 완료 알림 → 74행 마지막 상태 명령 → 81행 최종 반환을 확인했다. Agent 총 2회, 상태 조회 2회이며 Skill/Workflow/TaskOutput/SendMessage 호출은 없다. 메인 52행과 75행 lifetime은 4/4/0 → 12/12/0이다. 마지막 조회 이후 응답은 이 누계 밖이다. sessionRef는 af7826aaf01d1aa59d14852312f83b3e다.

이 실행의 메인과 부모 모델은 같다. 따라서 직접 부모와 최상위 구분은 [session-10](audit-2026-09-10-direct-parent-success.md)의 sol → terra → terra 증거와 결합한다.

## 요구사항별 대조

| 요구사항 | 현행 소스·로컬 증거 | 사용자 실행 증거와 판정 |
|---|---|---|
| 네 GPT 직접 선택 | clauduct.mjs sessionAgentDefinitions → agent-selection.mjs definedRoutes → native-gateway.mjs prepared.selected. test-agent-selection의 네 일반 정의 loopback 검사 | 9f4e7179: 일반 sol 요청 9~28의 관찰된 자식 요청, astra 43/44, luna 58/59 성공. 해당 metadata와 자식 완료 반환 재확인. terra는 당시 51 upstream/OTHER 실패였으며 db34be24의 11/12 성공으로 후속 정상 경로 확인. 과거 실패 원인이 소급 해결됐다는 뜻은 아님 |
| 명시 inherit 모델·effort | 부모 route 불변 복사, 20개 모델/effort 로컬 조합 | 2f96f1df terra/max, a2d50ff0 sol/high 두 단계 실제 성공 |
| 직접 부모와 생성 시점 | remember에 실제 prepared.selected 전달, 부모 객체 사후 변경·다른 메인 body·잘못된 parent header 로컬 검사 | db34be24는 최상위와 다른 직접 부모 선택을 입증. 실행 중 부모 설정 변경 후 기존 자식 유지의 실제 시험은 별도 미실행; 불변성은 로컬 증거 |
| 역할 기본값 보존 | MODELS/ROLE_MODELS 및 선택·실행기 검사 | 2f96f1df 일반 역할 luna/max·Plan sol/xhigh 성공을 원본 status로 재확인. Explore의 이번 감사 실제 원본 대조는 없음 |
| native 입력·metadata·반환 연결 | native model enum 확장 없이 subagent_type 선택. 등록 정의를 요청 본문에서 받지 않음 | 일반 GPT 정의와 inherit의 생성 호출·metadata·실제 route·반환 대조. 새 모델 enum 지원이나 모든 Task 경로를 주장하지 않음 |
| 잘못된 입력/관계 거부 | 미등록 이름, 잘못된 호출·역할·모델·부모·세션, snapshot 누락 및 완료 알림 재사용 로컬 검사 | 공격 입력을 실제 계정에서 재실행하지 않음. 정상 반환만으로 보안 통과를 대신하지 않음 |
| 전역 설정·임의 --agents 차단 | launcher-native의 원본 환경 불변·내장 역할 미교체·고정 정의·차단 옵션 검사 | 이번 작업에서 전역 설정·권한·hook 변경 없음. 모든 외부 설정 조합의 실제 격리를 주장하지 않음 |
| Claude 별칭·전체 목록 제외 | 기존 별칭 회귀는 보존하되 요구사항의 대체 증거로 사용하지 않음 | 이번 native 증거는 clauduct-*의 GPT 직접 선택. Claude 모델 목록 확장 없음 |

9f4e7179의 astra 자식 a16d00ec119b976ae와 luna 자식 ac12a62f53321a1a1은 각각 Read 1회와 완료 반환, sol 자식 abad01f8490f0e691은 Bash/Write/Read/Edit/Agent를 포함한 작업과 완료 반환이 기록돼 있다. 이는 해당 작업 증거이며 일반 agent의 모든 개발 도구 완전 검증을 뜻하지 않는다. 원본 본문·인증값·reasoning은 이 감사에 복사하지 않는다.

## 이번에 실행한 로컬 검사

Node --permission에서 저장소 읽기, fixture가 필요한 검사만 src 쓰기를 허용했다. 테스트의 외부 transport는 합성이고 실제 통신은 loopback이다. 실제 Claude 실행·외부 요청은 0이며 launcher 검사 실제 인증 조회도 0이다.

- node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-agent-selection.mjs: passed=true.
- node --permission --allow-fs-read=D:\AIDEV\Clauduct src/test-launcher-native.mjs: passed=true.
- node --permission --allow-fs-read=D:\AIDEV\Clauduct --allow-fs-write=D:\AIDEV\Clauduct\src src/test-completion-selection.mjs: 46 통과. --symlink는 실행하지 않음.

## 남은 항목과 다음 행동

Not verified: symlink/junction 동적 경계, 모든 모델/effort·실제 도구 조합, 추가 깊이·병렬 손자·장기 안정성. 생성 시점 불변성의 실행 중 모델 변경 조합은 로컬 검사와 실제 정상 상속 증거를 구분한다. 전체 native 동적 경로·provider/fallback·경로별 결과와 기존 보안/운영 잔여 항목은 [남은 검증](remaining-verification.md)에 보존한다.

Blocked by: 기존 symlink 및 인증된 자동 native 실행 거부를 유지한다. 허용 권한을 넓혀 통과시키지 않는다. 이 제약이 남아 있으므로 무조건적인 전체 완료를 선언하지 않는다.

다음은 남은 호출 경로와 보안 항목의 로컬 감사다. 성공한 네 모델·다단계 상속 시험을 다시 요청하지 않는다. 새로운 실제 실행이 필요한 차이를 확인하기 전에는 session-12를 만들지 않는다.
