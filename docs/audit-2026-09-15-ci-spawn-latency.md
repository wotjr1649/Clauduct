# 감사 — CI 러너의 프로세스 기동 지연과 테스트 바운드

2026-09-15. 판정: **간헐적 CI 실패는 테스트 로직의 결함이 아니라, 따뜻한 개발 머신 기준으로 잡힌 벽시계 바운드가 프로세스 기동을 17~40배 지연시키는 러너에서 정상 실행을 죽인 것이다.** 네 단계로 수정했고(PR #8·#9·#11·#10), 검증용 어서션은 하나도 바꾸지 않았다.

이 문서를 남기는 주 목적은 5장의 **남은 관찰 항목 2건**이다. 1~4장은 그 2건이 무엇을 뜻하는지 읽을 수 있게 하는 최소 맥락이다.

## 1. 증상 — 매번 다른 지점에서, 동시 실행 중 하나만

`on: push:`(브랜치 무제한)와 `on: pull_request:`가 함께 걸려 있어 커밋 하나가 전체 스위트를 두 번 동시에 돌렸고, **둘 중 하나만** 실패했다. 어느 쪽이 실패하는지는 고정이 아니다.

| 브랜치(커밋별) | push 런 | pull_request 런 |
|---|---|---|
| `docs/handoff-citation-exception` (구) | FAIL | pass |
| `docs/handoff-citation-exception` (신) | pass | FAIL |
| `test/size-the-ci-hang-guards` | pass | FAIL |

실패 지점도 매번 달랐다. 이것이 오래 "sse-fixed flake" 한 가지로 오인된 이유다.

## 2. 원인 — 러너가 프로세스 기동을 17~40배 지연시킨다

관측된 네 건 전부가 프로세스 기동을 감싼 벽시계 바운드였다.

| 실패 | 바운드 | 로컬 실측 | CI 관측 |
|---|---|---|---|
| `verification/test-dotnet-http-transport.mjs:82` (pwsh `--self-test`) | `runOnce` 12000 | 755/731/735ms | 12.65초에 시그널 사망 |
| `src/test-headless-run-root.mjs:25` (pwsh ×4, 인자 거부로 즉시 종료) | `spawnSync timeout: 30000` | 1863ms | `ETIMEDOUT` 31169ms |
| `sse-fixed` | `DotnetHttpProbe.cs` `normal` 5000 | — | `category=TIMEOUT, 6411ms` |
| `sse-fixed` | `DotnetHttpProbe.cs:77` `CancelAfter(7000)` | — | `category=LOCAL_CHECK_FAILED, 11361ms` |

위치는 **당시 CI 로그에 찍힌 줄 번호**다. 수정으로 이동했으며 현재 위치는 아래 본문에 적는다.

마지막 실행에서 첫 루프백 케이스에 도달하기까지 **26.2초**를 썼다(파일 시작 23:53:13.059 → 실패 23:53:50.638, 케이스 자체 11.4초 제외). 같은 구간이 로컬에서는 약 1초다.

`pwsh`가 특히 느린 이유의 일부는 `verification/manual-http-probe.ps1:25`가 매 실행마다 `Add-Type -Path DotnetHttpProbe.cs`로 런타임 Roslyn 컴파일을 한다는 점이다. 다만 그것만으로 31초를 설명하지 못하며, 러너 자체의 지연이 주 원인이다.

### 네 번째 실패가 다른 카테고리로 나온 이유

`DotnetHttpProbe.cs:81`의 `CancelAfter`(수정 전 값 7000)는 모드 예산(`:209`)과 별개인 **중첩 가드**다. 성공 경로의 파서 프로세스 기동이 여기 걸리면 취소 주체가 바깥 `deadline`이 아니므로 `:294`의

```csharp
catch (OperationCanceledException) when (deadline.IsCancellationRequested) { throw; }
```

가 매치되지 않고 `:295`의 일반 `catch`로 떨어져, **타임아웃이 로컬 검사 실패로 둔갑**한다. 11361ms ≈ HTTP 왕복 4초 + 내부 7초로 맞는다. 안쪽 가드가 바깥 정책을 가로챈 것이 결함이었다.

## 3. 수정 — 가드만 움직였다

| PR | 내용 |
|---|---|
| #8 | probe 실패 메시지에 카테고리와 경과시간을 넣어, 실패가 스스로 원인을 말하게 했다 |
| #9 | `runOnce` 12000→60000, 워커 90000→240000, 모드 예산 `normal` 5000→30000, pwsh를 띄우는 나머지 두 테스트 30000→120000 |
| #11 | `RunProcess` 7000→60000, 서버측 `headersTimeout`/`requestTimeout`/`setTimeout` 60000→180000, 케이스별 소켓 해제 대기 1초→10초 |
| #10 | `push: branches: [main]` — 커밋당 런 2개→1개 |

#8이 없었으면 #11은 불가능했다. "sse-fixed 실패"라는 이름만으로는 네 지점을 구분할 수 없었고, `category=LOCAL_CHECK_FAILED, 11361ms`라는 값이 나온 뒤에야 "가드가 죽인 게 아니라 probe가 반환했다"가 보였다.

### 검증 어서션은 그대로다

바꾼 값은 전부 행(hang) 방지 가드다. probe의 데드라인이 실제로 동작하는지는 다음 둘이 판정하며 손대지 않았다.

- `short-timeout` → `elapsedMs >= 100 && elapsedMs < 3000` (예산 150ms)
- `deadline-45s` → `elapsedMs >= 44000 && elapsedMs < 52000` (예산 45000ms)

`normal`은 타임아웃이 나면 안 되는 케이스라 어떤 어서션도 그 경과시간을 읽지 않는다. 애초에 측정이 아니라 가드였다.

`DotnetHttpProbe.cs`의 시간 상수는 전수로 다섯 개이며, 그중 무언가를 판정하는 것은 모드 예산뿐이다 — `:81` 가드(수정함), `:97` kill 이후 `WaitForExit(2000)`, `:248` `Timeout.InfiniteTimeSpan`(토큰에 위임), `:231` `ResponseDrainTimeout = TimeSpan.Zero`(지연 아님), `:209` 모드 예산.

## 4. 검증과 그 한계

수정 후 로컬 전수: 스텝1 `tests 130 / pass 129 / fail 0 / skipped 1`(67.2초), 스텝2 9/9, 스텝3 6/6. `test-dotnet-http-transport.mjs`는 49.4초이며 이는 `deadline-45s`가 의도적으로 45초를 쓰기 때문이다.

**이 통과가 확인해주는 것은 회귀가 없다는 것뿐이다.** 이 결함은 처음부터 로컬에서 재현된 적이 없다 — 러너가 프로세스 기동을 지연시킬 때만 나타나며 로컬은 그 조건을 만들지 못한다. 필터를 끈 상태의 로컬 5회 연속 실행도 35개 루프백 케이스를 전부 통과했다.

### 배제한 가설

- **`POWERSHELL_UPDATECHECK=Off`** — [공식 문서](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_update_notifications?view=powershell-7.6)상 업데이트 체크는 시작 3초 **후** 비동기로 돌고 알림은 다음 실행에 표시된다. 기동을 블로킹하지 않으므로 원인이 아니다. 적용하지 않았다.
- **node 버전 핀** — `DotnetHttpProbe.cs:185`의 `v24.19.0` 요구는 `Live()` 경로 안에만 있어 루프백 테스트가 지나지 않는다. CI가 v24.20.0인 것은 무관하다.
- **Defender 실시간 검사** — 호스티드 Windows 러너의 알려진 원인이지만, 보안 통제를 꺼서 테스트를 통과시키는 방향이라 적용하지 않았다.

### 정정한 판독 세 가지

1. push/pull_request 중 실패하는 쪽이 고정이라고 적었으나 틀렸다. 런 순서로 이벤트를 추측한 결과였고, 실제로는 무작위다(1장 표).
2. `POWERSHELL_UPDATECHECK` 가설을 유력하다고 적었으나 공식 문서로 반증됐다.
3. #9 시점에 "실제로 pwsh를 띄우는 3곳이 전부이며 다음 라운드는 없다"고 적었으나 틀렸다. 그것은 *프로세스를 시작하는 지점*의 집합이었지 *경로에 있는 바운드*의 집합이 아니었고, `RunProcess`의 7000은 probe 내부에 중첩돼 있었다.

## 5. 남은 관찰 항목 2건

### 5-1. flake 재발 여부 — 미판정

#11 이후 CI가 연속 녹색이지만 **간헐 결함이므로 이것은 해결의 증명이 아니다.** 로컬이 판정할 수 없는 종류이므로 CI에서 누적 관찰해야 한다.

관찰 조건이 이전보다 낫다:

- #10으로 커밋당 런이 1개가 되어 신호가 깨끗하다. 이전에는 같은 커밋에 두 런이 떠서 "하나만 실패"가 정상처럼 보였다.
- #8의 진단이 붙어 있어, 재발 시 실패 메시지가 카테고리와 경과시간을 스스로 말한다.

**재발 시 할 일**: 우회하거나 재실행으로 넘기지 말고, 메시지의 `category=`와 경과시간, 그리고 해당 파일 블록의 시작~실패 타임스탬프 차이를 먼저 기록한다. 그 값이 어느 바운드에 걸렸는지 3장의 표와 대조하면 위치가 나온다. 새 지점이면 그 바운드가 가드인지 검증 대상인지부터 구분한다 — 검증 대상이면 넓히지 않는다.

### 5-2. `deadline-45s` 상한 7초 여유 — 의도적으로 두었다

`verification/test-dotnet-http-transport.mjs:215`:

```js
if (testCase.mode === 'deadline-45s') assert(elapsedMs >= 44000 && elapsedMs < 52000);
```

45초 데드라인에 상한 여유가 7초뿐이다. 이 러너는 프로세스 기동에서 17~40배 지연을 보였으므로, 지연이 이 케이스 도중에 발생하면 **어서션이 정당하게 실패할 수 있다.**

넓히지 않은 이유: 이것은 가드가 아니라 **검증 대상**이다. 하한 44000은 "일찍 포기하지 않았다", 상한 52000은 "아예 타임아웃이 안 난 게 아니다"를 각각 확인한다. 임의로 넓히면 후자가 약해진다.

**재발 시 선택지**(어느 것도 아직 채택하지 않음):

| 선택 | 대가 |
|---|---|
| 상한을 넓힌다 | "아예 타임아웃이 안 났다"를 잡는 힘이 약해진다 |
| 케이스 예산을 45초보다 짧게 재설계한다 | 긴 데드라인 경로의 검증 범위가 줄어든다 |
| 경과시간 기준을 probe 내부 측정으로 바꾼다 | 테스트가 probe의 자기보고에 의존하게 된다 |

판단에 필요한 것은 실제 실패 1건의 `elapsedMs`다. 그 값이 52000을 얼마나 넘는지에 따라 선택이 갈린다.

## 참고

- `docs/verification-modes-2026-09-14.md` — CI 도입에서 드러난 환경 가정과, 이 문서와 별개로 열려 있는 `poc/test-claude-read-once.mjs`의 CI 전용 실패
