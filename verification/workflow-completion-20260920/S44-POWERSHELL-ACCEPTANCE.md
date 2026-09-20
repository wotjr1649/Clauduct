# S44 PowerShell 5.1·7 임시 정책 실행 검수

2026-09-21 KST. **승인된 공개 시험의 PowerShell 5.1·7 실행, 자식 실행, native
TaskStop 회수, 같은 세션 후속 응답은 합격이다.** 사용자·시스템 영구 실행 정책을
변경하지 않았다. 모든 PowerShell 스크립트나 모든 종료 환경의 합격으로 확대하지 않는다.

## 대상과 원인

- 실제 native TUI UUID: `7055dbfa-bec9-4880-98d6-a32d6f35ecca`.
- Native `2.1.278`; 개발 `clauduct.exe` SHA256
  `7feb620944578d85982f8ecac20fe3ffba0096207da0e9b81682f603b5b1036f`.
- 제품 source `31ff1184c21d7dac0fccd03394081aacd78b9db5`, 제품 바이너리 변경 없음.
- 공개 시험 폴더는 이전에 승인된 `interactive-e268b95d-2048-47e3-b36b-5483000541a7/project`.
- 사용자 승인: 두 버전의 해당 시험 프로세스와 시험 자식에 한정한 임시 실행 정책 적용.

Codex 재시작 후 호스트의 PowerShell 실행 거부가 해소되어 실제 정책 조회가 가능해졌다.
Windows PowerShell 5.1은 모든 범위가 Undefined이고 유효 정책이 Restricted였다.
PowerShell 7은 LocalMachine이 RemoteSigned이며 유효 정책도 RemoteSigned였다.
따라서 Codex 실행 규칙과 Windows의 파일 실행 제한은 별개의 조건이었다.

또한 기존 공개 probe가 자식 경로를 `$PSHOME/powershell.exe`로 고정했다. 이는 PowerShell
7의 `pwsh.exe`에 맞지 않아, 실행 중인 프로세스의 `MainModule.FileName`을 사용하도록
[probe](public-s44b-probe.ps1)를 수정했다. 부모와 자식이 기록하는 버전·엔진 경로·유효
정책·Process 정책을 추가했다. 이 결함은 실행 전 코드 검수로 발견했다.

## 실제 실행

두 시험 모두 Workflow의 단일 B 자식 `gpt-5.6-terra/medium`, 도구 `[Bash]`를 사용했다.
foreground Bash timeout은 240000ms이며 probe의 자연 대기는 부모·자식 각각 180초다.
각 Workflow와 Bash 명령에 native 일회성 실행 승인을 사용했다.

| 대상 | 실행 명령의 엔진 | probe ID | task / Workflow |
|---|---|---|---|
| Windows PowerShell 5.1.26100.8870 | `C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe` | `s44b-510000000001` | `wzbff9l19` / `wf_b043a1af-4ab` |
| PowerShell 7.6.6 | `C:/Program Files/PowerShell/7/pwsh.exe` | `s44b-700000000001` | `wwxvifxt5` / `wf_16649c64-5cb` |

루트 인자는 `-NoProfile -ExecutionPolicy RemoteSigned -File ./public-s44b-probe.ps1
-ProbeId <해당 ID>`다. 자식은 같은 엔진으로 같은 공개 파일을 `-Leaf`로 실행하며 부모의
Process 정책을 상속했다. 부모·자식 4개 모두 유효 정책과 Process 정책이 RemoteSigned임을
실제 marker에서 확인했다. `Set-ExecutionPolicy`, 전역 allow, auto mode는 사용하지 않았다.

## OS 실행·회수와 회복

[관측기](observe-s44b.ps1)는 각 marker의 PID·시작 시각을 OS와 대조하고 프로세스 핸들을
유지한다. CIM의 부모 관계도 대조한다. 취소 후 같은 핸들의 `HasExited`와 `ExitTime`을
읽으므로 단순 PID 목록 부재로 회수를 추정하지 않는다. 관측기는 프로세스를 종료하지 않는다.

| 대상 | 실제 root / leaf PID | 마지막 OS 종료(UTC) | TaskStop 호출→마지막 종료 |
|---|---|---|---:|
| 5.1 | 18924 / 4480 | 2026-09-20T15:33:13.6009287Z | 188ms |
| 7 | 18916 / 19024 | 2026-09-20T15:36:01.6315814Z | 167ms |

두 시험 모두 먼저 실제 실행을 관측한 후 TaskStop을 호출했다. 180초 자연 종료 이전이며
finished marker는 0개다. 각 취소 후 `PS51_STOPPED=53` / `PS7_STOPPED=53`, 그다음 별도
새 사용자 입력의 `PS51_RECOVERY=71` / `PS7_RECOVERY=71` 답변을 확보했다. 두 B 실행 모두
한 번뿐이며 자동 재실행하지 않았다. 이 지연은 두 관측값이지 통계적 상한이 아니다.

합계 27 requests, API 실패 0, native 도구 실패 0, 결과 미확보 0, 요청 취소 0,
의도한 native 취소 2. 정상 exit 0, watchdog 개입 없음, native 회수 완료다.

시험 제어 중 PTY에 보낸 CR/LF가 즉시 제출되지 않고 입력창에 남은 경우가 있었다.
5.1에서는 추가 LF 뒤 제출됐으며 7에서는 명시적 Enter 키 시퀀스 `ESC[13u`로 제출됐다.
이를 자동 재전송이나 제품 API 실패로 계산하지 않는다. 각 B 생성 요청과 실제 Bash는
한 번이며, 이 현상의 모든 터미널 원인까지 확정하지 않았다. OS 회수 지연은 입력 시도부터가
아니라 transcript의 실제 TaskStop tool_use부터 계산했다.

## 정책 복원과 검사

새 프로세스에서 읽은 시험 전후 정책의 모든 범위를 비교해 완전히 같음을 확인했다.
5.1의 기본 유효 정책은 Restricted, 7은 RemoteSigned, 둘 다 Process는 Undefined다.
그러므로 **5.1의 일반 `.ps1` 실행을 영구적으로 허용한 상태는 아니다.** 승인된 이번
시험 프로세스만 임시로 실행 가능하게 한 결과다.

- [시험 전 정책](powershell-20260921/policy-before.json)
- [시험 후 정책](powershell-20260921/policy-after.json)
- [두 엔진 문법·잘못된 ProbeId 거부 검사](powershell-20260921/preflight.json)
- [5.1 실행 근거](s44b-510000000001/before.json), [회수 근거](s44b-510000000001/after.json)
- [7 실행 근거](s44b-700000000001/before.json), [회수 근거](s44b-700000000001/after.json)
- [최종 대조 결과](powershell-20260921/acceptance.json)

다음 명령은 보관된 근거를 검산하며 workload를 재실행하거나 정책을 바꾸지 않는다.

```text
node verification/workflow-completion-20260920/review-s44b-powershell.mjs
```

15개 근거 검사가 통과했다. 실제 대화형 시험은 2건이며 15회 실행했다고 계산하지 않는다.
새 대조 스크립트 문법, 두 엔진의 probe 문법·잘못된 ID 거부를 검사했다. 제품 코드가
바뀌지 않아 전체 Go 회귀를 다시 실행하지 않았다. 앞선 Restricted 실패 기록은 그대로 보존한다.

이 합격은 [기존 B OS 수용](S44-B-OS-ACCEPTANCE.md)에 PowerShell 두 버전의 근거를
추가한다. 최종 v0.3.0 버전 반영·배포물 빌드·설치 검증은 별도이며 공개 출시는 하지 않았다.

Process 정책의 수명과 자식 상속 근거:
[Microsoft about_Execution_Policies](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_execution_policies?view=powershell-7.6).
