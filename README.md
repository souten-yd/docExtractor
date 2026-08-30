# docExtractor

QNAP NAS 上で漫画・ドキュメント系アーカイブを、**中間展開を極力行わず、書き換えを最小化して整理する**軽量 Web アプリです。

初期ターゲットは **QNAP TS-253Be / x86_64 / QTS 5.x / 16 GB RAM** です。元の `mangaOrganize.py` の処理目的を引き継ぎつつ、2 GB 以上のアーカイブ、並列処理、障害復旧、Web 管理、診断ログを前提に再設計しています。

## 現在の処理方式

- `.zip` / `.rar` を対象フォルダから検出
- ファイル名から作者・シリーズ名・巻数を推定し、実行前に Web UI でプレビュー
- 正常な ZIP は**再圧縮せず**、同一ファイルシステム上の `rename()` だけでシリーズフォルダへ整理
- RAR はディスクへ全展開せず、固定 RAM バッファで **RAR decoder -> ZIP writer** をストリーミング
- ZIP/RAR 内の ZIP/RAR/CBZ/CBR は再帰的に正規化し、最終 ZIP 内に入れ子アーカイブを残さない
- 再帰正規化後の最上位フォルダごとに ZIP を分割するため、複数巻入りアーカイブは原則 1 巻 1 ZIP になる
- dry-run は内包アーカイブまで読み取り、実行時と同じ最終分割先を表示する
- 出力は各 `<name>.zip.partial` に作成し、全出力の検証後に atomic rename。その後にのみ元 RAR を削除
- 既圧縮画像（JPEG/PNG/WebP/AVIF 等）は ZIP STORE を優先して J3455 の CPU 負荷を抑制
- 2 worker を標準、実装上は 1～3 worker に制限
- 1 worker あたりの RAR dictionary は標準 512 MiB 上限、ストリームバッファは標準 8 MiB
- RAR 変換前に出力先の空き容量を事前確認

## Web 管理画面

QPKG 起動後、QTS のアプリ画面から開きます。QTS HTTP Proxy 経由の `/docExtractor` と直接ルートの両方を処理できます。

Web UI から以下を実行できます。

- 処理対象フォルダの設定・保存
- `/share` 配下を辿るフォルダピッカー
- 対象フォルダのスキャン
- シリーズ名・巻数・信頼度・予定処理の確認
- 安全判定されたファイルだけ実行
- 低信頼度ファイルを含めた明示実行
- ジョブ進捗、処理 stage、worker、待ち時間、経過時間の確認
- ジョブ別 Read / Write / 合計実効 I/O (MiB/s) の確認
- 実行中 / 待機 / 成功 / 失敗、現在速度、完了ジョブ平均速度のサマリー表示
- NAS の 1 分 load average / CPU 数、空き RAM の簡易表示
- 実行中ジョブのキャンセル
- ジョブログの画面表示 / JSONL ダウンロード
- ジョブ単位または全体の診断 ZIP ダウンロード
- GitHub Releases の最新版確認と Web からの QPKG 更新
- 更新パッケージのダウンロード進捗、検証状態、更新ログの確認

保存した対象フォルダは即時反映され、QPKG 再起動・アップデート後も `/etc/config/docExtractor.settings.json` から復元されます。Web UI から設定できる範囲は `/share` 配下に制限します。

## Web 更新

**v0.2.0 以降**は画面右上の「更新」から最新版を確認・適用できます。v0.1.x から v0.2.0 への移行だけは App Center から v0.2.0 QPKG を一度手動で上書きインストールしてください。それ以降のリリースは Web 更新を利用できます。

更新処理は次の安全条件を満たした場合だけ実行します。

1. GitHub の `souten-yd/docExtractor` 最新 Release だけを確認
2. Release バージョンと一致する `docExtractor_<version>_x86_64.qpkg` だけを選択
3. QPKG サイズを上限 100 MiB に制限し、メモリへ全読み込みせずストリーミング保存
4. GitHub Release asset が公開する SHA-256 とダウンロード後の SHA-256 を照合
5. SHA-256 不一致なら QPKG を削除し、インストールを開始しない
6. 実行中または待機中のアーカイブジョブがある場合は更新を拒否
7. 検証完了後だけ QPKG インストーラを開始し、Web UI は一時切断後の再接続を自動試行

更新失敗時は「更新ログ」からインストーラ出力を確認できます。Web 更新は `X-docExtractor-Confirm` を使った同一オリジンの明示操作を要求し、通常のフォーム送信だけでは開始できません。

## 速度計測 / ログ / デバッグ

速度調整は Web UI だけで判断できるよう、ジョブ開始からの平均 Read / Write / 合計 I/O MiB/s と経過時間を表示します。ZIP の `rename()` のみで完了するジョブは大容量ファイルでも実データを書き直さないため、速度集計から除外します。

ジョブログは JSONL 形式です。ログ書き込み自体が I/O 負荷にならないよう、通常の progress は stage 変更時または約 256 MiB ごとに記録します。加えて、各 stage の完了時には所要時間、Read / Write 量、stage 実効 I/O MiB/s を記録します。最終イベントには処理方式（再帰正規化・フォルダ分割・ZIP rename/copy など）、entry 数、総所要時間、平均速度が入ります。

診断 ZIP にはアーカイブ本体や画像を入れません。安全な範囲で、直近ジョブログ、docExtractor/Go/OS 情報、QTS 情報、メモリ情報、対象ストレージ空き容量などを格納します。パスは basename 化し、token/password/secret 系フィールドはマスクします。ジョブログの標準保持期間は 14 日です。

実機の並列数を決める際は、同じデータセットで worker=1 と worker=2 を比較し、Web UI の完了ジョブ平均 I/O と NAS load を確認してください。TS-253Be では 16 GB RAM より J3455 CPU / ストレージ I/O が先に上限になる想定です。

## QNAP アイコン

QPKG には QTS / App Center 用の専用アイコンを同梱します。

- `qpkg/icons/docExtractor.png`: 64×64
- `qpkg/icons/docExtractor_80.png`: 80×80
- `qpkg/icons/docExtractor_gray.png`: 無効状態用 64×64

小さい表示でも識別しやすい、アーカイブ／取り込みを表すシンプルなデザインです。

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

通常の PR は Linux 1 ジョブだけで `go test ./...`、`go vet ./...`、静的 x86_64 build を実行します。OS/Go matrix、npm build、PR artifact は使いません。同じブランチの古い CI は自動キャンセルします。

QPKG の生成は **リリース、明示した PR 検証、または手動実行時だけ**です。Release 時は QPKG を GitHub Release に直接添付し、Actions Artifact を重複保存しません。PR/手動ビルドの Artifact は 1 日で削除します。

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
