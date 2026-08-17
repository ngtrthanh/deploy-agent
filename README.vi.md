# deploy-agent — hướng dẫn nhanh

`deploy-agent` (DA) là **một binary native rất nhỏ** chạy trên host. DA không chạy trong Docker; nó dùng Docker/Compose để giữ app đúng phiên bản mong muốn.

## 1. Tải binary

Linux / macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/ngtrthanh/deploy-agent/main/scripts/get.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/ngtrthanh/deploy-agent/main/scripts/get.ps1 | iex
```

Mặc định tải rolling release `edge`. Muốn pin bản ổn định:

```sh
DA_VERSION=v0.2.0 sh scripts/get.sh
```

```powershell
$env:DA_VERSION="v0.2.0"; .\scripts\get.ps1
```

Binary hiện được build sẵn trên GitHub cho:

- Linux: amd64, arm64, armv7, armv6, 386
- Windows: amd64, arm64, 386
- macOS: amd64, arm64

Mỗi file được kiểm SHA-256 khi tải.

## 2. DA làm gì?

```text
desired version
      ↓
DA kiểm app đang chạy
      ↓
khác version → pull → docker compose up
      ↓
app /healthz đúng version → ACCEPT
      ↓
sai → rollback bản trước
```

Production nên chạy DA kiểu **one-shot** bằng scheduler của HĐH:

- Linux: systemd timer
- Windows: Task Scheduler
- macOS: launchd (sẽ bổ sung installer)

Vì scheduler gọi lại DA định kỳ, DA tự sống lại sau reboot/crash mà không cần một daemon riêng.

## 3. Chạy demo Docker

Máy có Docker + Go:

```sh
mkdir -p bin
go build -o bin/deploy-agent ./cmd/deploy-agent
bash demo/run.sh
```

Demo tự làm đủ vòng:

```text
start local registry
→ build app v1
→ DA deploy v1
→ build/publish v2
→ DA update v2
→ verify /healthz
→ docker compose down
→ xóa registry + temp state
```

GitHub Actions chạy demo này trên mọi push/PR để bảo đảm luồng deploy thực sự hoạt động và cleanup sạch.

## 4. File chính

- `scripts/get.sh`, `scripts/get.ps1`: tải đúng binary theo OS/CPU.
- `packaging/systemd/`: chạy survive reboot trên Linux.
- `scripts/install-windows.ps1`: Task Scheduler trên Windows.
- `demo/`: Docker app demo end-to-end.
- `docs/`: deployment và `/healthz` contract.

> DA hiện là v0.x. Bước tiếp theo là chuyển canonical deployment identity từ Git SHA sang image digest theo STD-CICD v2.
