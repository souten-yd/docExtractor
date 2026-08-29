# docExtractor

QNAP NAS 上で漫画・ドキュメント系アーカイブを、**中間展開を極力行わず、書き換えを最小化して整理する**軽量 Web アプリです。

初期ターゲットは **QNAP TS-253Be / x86_64 / QTS 5.x / 16 GB RAM** です。元の `mangaOrganize.py` の処理目的を引き継ぎつつ、2 GB 以上のアーカイブ、並列処理、障害復旧、Web 管理、診断ログを前提に再設計しています。

## 現在の処理方式

- `.zip` / `.rar` を対象フォルダから検出
- ファイル名から作者・シリーズ名・巻数を推定し、実行前に Web UI でプレビュー
- 正常な ZIP は**再圧縮せず**、同一ファイルシステム上の `rename()` だけでシリーズフォルダへ整理
- RAR はディスクへ全展開せず、固定 RAM バッファで **RAR decoder -> ZIP writer** をストリーミング
- RAR の中身が ZIP 1 個だけなら、その ZIP を直接取り出して終了（再圧縮なし）
- RAR 内に画像と ZIP が混在する場合、内側 ZIP は通常ファイルとして保持し、再帰展開しない
- 出力は `<name>.zip.partial` に作成し、検証後に atomic rename。その後にのみ元 RAR を削除
- 既圧縮画像（JPEG/PNG/WebP/AVIF 等）は ZIP STORE を優先して J3455 の CPU 負荷を抑制
- 2 worker を標準、実装上は 1～3 worker に制限
- 1 worker あたりの RAR dictionary は標準 512 MiB 上限、ストリームバッファは標準 8 MiB
- RAR 変換前に出力先の空き容量を事前確認

## Web 管理画面

QPKG 起動後、QTS のアプリ画面から開きます。QTS HTTP Proxy 経由の `/docExtractor` と直接ルートの両方を処理できます。

Web UI から以下を実行できます。

- 対象フォルダのスキャン
- シリーズ名・巻数・信頼度・予定処理の確認
- 安全判定されたファイルだけ実行
- 低信頼度ファイルを含めた明示実行
- ジョブ進捗、read/write 量、処理 stage の確認
- 実行中ジョブのキャンセル
- ジョブログの画面表示 / JSONL ダウンロード
- ジョブ単位または全体の診断 ZIP ダウンロード

## ログ / デバッグ

ジョブログは JSONL 形式で保存します。ログ書き込み自体が I/O 負荷にならないよう、進捗は stage 変更時または約 256 MiB ごとに記録します。

診断 ZIP にはアーカイブ本体や画像を入れません。安全な範囲で、直近ジョブログ、docExtractor/Go/OS 情報、QTS 情報、メモリ情報などを格納します。パスは basename 化し、token/password/secret 系フィールドはマスクします。ジョブログの標準保持期間は 14 日です。

## QPKG 設定

QPKG の標準設定は `qpkg/shared/docExtractor.conf` です。

```sh
LISTEN="127.0.0.1:8765"
ROOT="/share/Download/Temp"
WORKERS="2"
BUFFER_MIB="8"
MAX_DICT_MIB="512"
COMPRESSION="balanced"
```

QPKG では Web サービスを localhost のみに bind し、QTS Proxy から公開する構成です。

圧縮モード:

- `fast`: ZIP entry を原則 STORE。最速・CPU 最小
- `balanced`: 画像や既圧縮形式は STORE、その他は軽い Deflate（標準）
- `compact`: 非圧縮形式を通常 Deflate。容量優先

## GitHub Actions / コスト方針

通常の PR は Linux 1 ジョブだけで `go test ./...` と `go vet ./...` を実行します。OS/Go matrix、npm build、PR artifact は使いません。同じブランチの古い CI は自動キャンセルします。

QPKG の生成は **`v*` タグまたは手動実行時だけ**です。タグ時は QPKG を GitHub Release に直接添付し、Actions Artifact を重複保存しません。手動ビルドの Artifact は 1 日で削除します。

詳細は [`docs/ci-cost.md`](docs/ci-cost.md) を参照してください。

## 開発

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o docExtractor ./cmd/docextractor
```

ローカル起動例:

```sh
./docExtractor --root /path/to/test-library --workers 2
```

実データで検証する際は必ずバックアップを用意し、最初はコピーしたテストデータを使用してください。
