# Blockstep

lct의 사용자 시나리오가 현재 패키지에서 어디까지 동작하고 어느 스텝에서 막혔는지, 막힌 원인 이슈를 누가 언제까지 처리하는지 한 곳에서 보여 주는 도구다. 플랫폼팀과 엔진팀이 함께 쓴다.

- 설계안: [docs/plan/blockstep-plan.md](docs/plan/blockstep-plan.md)
- UI 초안: Design 캔버스 「Blockstep UI 초안」 https://claude.ai/artifact/52dFYtU8Hw7SAFx28qNPo5 (공유된 사람만 열 수 있다)

## 구성

- Go `net/http`와 `html/template`로 서버에서 HTML을 만든다. 노선도는 서버가 SVG로 만든다
- 저장은 SQLite 파일 하나다. 시작할 때 스키마를 적용하므로 파일이 없으면 만든다
- 실행 파일 하나를 사내 서버에서 실행한다

| 패키지 | 하는 일 |
|---|---|
| `cmd/blockstep` | 실행 파일. `-db`, `-addr` 인자를 받는다 |
| `internal/store` | SQLite 스키마와 질의, 상태 전환 규칙 |
| `internal/outline` | 아웃라인 텍스트와 스텝 트리 사이의 변환 |
| `internal/railmap` | 노선도 SVG 생성 |
| `internal/notify` | Slack Incoming Webhook 전송 |
| `internal/web` | HTTP 핸들러와 템플릿 |

## 실행

```bash
go run ./cmd/blockstep -db blockstep.db -addr :8080
```

- `-db`: SQLite 파일 경로. 기본값 `blockstep.db`
- `-addr`: 듣는 주소. 기본값 `:8080`

실행 파일로 배포하려면 `go build -o blockstep ./cmd/blockstep`로 만든 파일과 SQLite 파일만 두면 된다. Windows에서는 `.build.ps1`이 같은 명령으로 `blockstep.exe`를 만든다.
Slack 알림은 화면의 「설정」에서 Incoming Webhook URL을 넣어야 보낸다. 비어 있으면 보내지 않는다.

## 검증

```bash
go build ./... && go vet ./... && go test ./...
```

## 줄끝

새로 만드는 파일은 LF로 쓴다. `.gitattributes`가 저장소에 담기는 쪽을 고정한다.
