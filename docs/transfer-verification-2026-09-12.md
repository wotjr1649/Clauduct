# 디렉터리 이관 검증 기록

작업 재개 진입점은 [HANDOFF.md](../HANDOFF.md), 장기 무인 개발의 전체 제안은 [심층 리서치](research-2026-09-12-unattended-release-gates.md)다. 기록일은 2026-09-12, 원본은 `D:/AIDEV/Clauduct`, 코드 HEAD는 `aa317c75c7bad0cf9d16641ca7db4edb43016c19`이다.

## 검사 결과

| 검사 | 관측 | 판정 범위 |
|---|---|---|
| 시작 상태 | tracked·staged 변경 없음, 기존 사용자 untracked 자료와 리서치 문서 존재 | 사용자 상태를 보존하며 새 문서를 추가 |
| 코드 이력 | main과 release linked worktree 모두 `aa317c7` | 이전 작업의 제품 변경은 원래 루트에 반영됨 |
| 주 `.git` | 실제 디렉터리, object alternates·commondir·submodule 파일 없음 | 확인한 Git 구조에 외부 object 디렉터리 의존성 없음 |
| linked worktree | main→linked, linked→main 모두 `D:/AIDEV/Clauduct` 절대경로 | 다른 경로로 복사 시 보조 연결 복구가 필요할 수 있음 |
| 프로젝트 경로 순회 | 파일 메타데이터 순회에서 reparse point 0, 읽기 오류 0 | 당시 디렉터리 목록 기준. 악성 링크 경합 방어 시험이 아님 |
| 실행기 | root/bin의 cmd가 자기 위치 기준으로 `src/clauduct.mjs`를 선택 | 이동 가능한 상대 진입점 |
| 도구 위치 | `src/runtime-paths.mjs`가 OS 사용자 홈으로 계산 | 사용자명이 같을 필요보다 해당 위치·로그인이 실제로 존재하는지가 중요 |
| 프로젝트 문서 | 리서치 344행, 장애 ID F01~F23, 출처 19개 | 연구 내용을 같은 디렉터리에 보존 |
| Claude auto memory | 사용자 설정 `autoMemoryEnabled=false`, custom memory directory 없음, 현재 config-dir override 없음 | 현재 사용자 설정만 확인. 관리형·과거 실행별 override 전수 감사는 아님 |
| Claude project memory | Clauduct 관련 기본 project 6개와 프로젝트 profile에서 memory 문서 없음 | 확인한 project 범위에서 native memory 이관 대상 없음 |
| Claude 사용자 agent memory | 두 기본 agent memory 디렉터리 없음 | 기본 사용자 위치의 추가 저장소 없음 |
| Codex native memory | `memories_1.sqlite`의 `stage1_outputs` 0행, `jobs` 0행 | read-only SQL count. memory 본문을 복사하지 않음 |
| Codex 파일형 memory 후보 | 기본 세 후보 위치 없음 | 모든 가능한 plugin 저장소의 부재 증명은 아님 |
| 전역 Claude 지침 | 존재, `Clauduct` 문자열 없음 | 환경 지침과 프로젝트 이관 지식 구분 |
| 세션 이력 | Claude project JSONL·자식 디렉터리와 Codex 대상 세션이 사용자 홈에 존재 | 폴더 복사만으로 동일 대화 resume 보장 불가 |

## 릴리즈 이동 실행 확인

검증한 기존 ZIP:

```text
.tmp/release-2026-09-12/.tmp/release-artifacts-fc09f62979424ad289b01eab3d27b765/Clauduct-e216ad883728.zip
SHA256 191a6654e92aee3785bda7a31df2553d381784951dedaa479e4aabfaa8e553b8
```

ZIP에는 파일 68개와 디렉터리 entry 7개, 총 entry 75개가 있었다. 파일 비압축 크기 합계는 1,021,692 bytes다. 예상 SHA256, 상대경로·부모 이동 부재·중복 경로 부재·symlink entry 부재·크기 상한을 확인한 뒤 새 디렉터리에 풀었다.

현재 원본과 배포 파일의 내용 68개가 일치했다. 25개는 바이트 단위 동일하고 43개는 CRLF/LF 차이만 있었다. 검증 과정의 초기 assertion 두 건은 디렉터리 entry를 파일 수로 센 가정, checkout과 ZIP의 줄바꿈이 같다는 가정에서 발생했다. 두 가정을 관측에 맞게 정정했고 제품 소스는 수정하지 않았다. 다른 내용 차이는 허용하지 않았다.

새 로컬 검사 경로:

```text
.tmp/transfer portability 20260912/Clauduct
```

그 위치에서 실행했다.

```powershell
.\clauduct.cmd --dry-run -p
```

| 값 | 결과 |
|---|---|
| exit code | 0 |
| mode | print |
| model / effort | gpt-6-astra / low |
| context | 400000 / compactAt 320000 |
| credentialReads | 0 |
| childStarted | false |
| globalWrites | 0 |

이는 **같은 머신의 다른 공백 경로에서 실행기·모듈 경로가 해석됨**을 확인한 것이다. 다른 머신의 인증·도구 설치·backend 성공을 대신하지 않는다. 검사 디렉터리는 작업 전용 `.tmp` 하위에 남겼다. 개발 맥락의 이관은 이 배포본이 아니라 원본의 docs와 `.git`를 포함한 프로젝트 디렉터리를 기준으로 한다.

## 전달 범위의 한계

기존 대화 없이 `HANDOFF.md`만 시작점으로 삼는 조건을 추가 검사했다. 연결되는 Markdown 문서 44개를 재귀 확인하여 로컬 링크 누락 0·프로젝트 외부 로컬 링크 0을 확인했다. 웹 출처 URL은 38개이며 본문 전체가 오프라인 보관돼 있다는 뜻은 아니다.

`src`, `poc`, `verification`의 실행·검증용 `.mjs`와 정적 상대 import로 이어지는 67개 모듈을 확인했다. 문자 그대로 명시된 상대 import 171개에서 누락·프로젝트 외부 참조는 없었다. 이 검사는 임의 동적 경로·생성 fixture·OS 설치 파일의 전체 검증이 아니다. 테스트 파일 존재는 `src` 23 mjs + 1 ps1, `poc` 6 mjs, `verification` 4 mjs를 확인했으며 이 추가 점검에서 실행하지 않았다.

F01~F23은 리서치의 향후 장애 시험 명세이며 모두 구현됐다고 표시하지 않는다. 기존 테스트와 신규 명세를 대응시켜 구현·검증하는 것이 다음 개발 작업이다. 원래 대화의 raw transcript 없이도 이 단계부터 시작할 수 있도록 HANDOFF에 명시했다.

기존 68파일 ZIP은 실행 배포물이다. Git 이력·현행 감사·최신 리서치·HANDOFF 문서·사용자 작업 상태를 포함하는 개발 이관 묶음이 아니다. 폴더 전체 복사와 clone·git archive·릴리즈 ZIP을 같은 것으로 취급하지 않는다.

폴더 전체 민감정보 점검은 자격증명 경로 이름이 포함돼 자동 승인 검토 `shell-guard`에서 실행 전 차단됐다. 해당 검사는 재구성하여 우회하지 않았다. 메모리 위치의 독립 설정 필드와 DB count, 코드·문서·검증된 ZIP의 내용 대조는 별도 허용된 읽기로 확인했다. 인증 값·세션 원문·호스트 DB는 새 문서나 프로젝트에 복사하지 않았다.

대상 머신 전송·외부 업로드·push·인증 변경·host 설정 변경·worktree repair는 수행하지 않았다. 문서 내용의 완결성과 ‘실제로 다른 물리 머신에서 통과했다’는 주장은 구분한다.

## 출처

- [Claude Code Memory](https://code.claude.com/docs/en/memory): auto memory의 로컬 저장과 설정 의미.
- [Claude Code Sessions](https://code.claude.com/docs/en/sessions): transcript 저장과 resume.
- [Git worktree](https://git-scm.com/docs/git-worktree): main/linked worktree가 함께 이동했을 때의 repair.
- [기존 릴리즈 검증](release-readiness.md), [현행 검증표](remaining-verification.md), [실행 파일 위치](../src/runtime-paths.mjs), [주 실행기](../src/clauduct.mjs).
