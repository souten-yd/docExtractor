# アプリアイコン

iPhoneのホーム画面とQNAP App Center／デスクトップは、書類・フォルダ・抽出矢印の共通デザインを使用します。

## iPhone

更新後、SafariでdocExtractorを開き、共有 →「ホーム画面に追加」を選択してください。既存のホーム画面アイコンが変わらない場合は、そのショートカットを削除して再追加してください（NAS上のデータは削除されません）。

180×180の不透明PNGを`apple-touch-icon`で指定しています。ホーム画面名は`docExtractor`です。Webアプリのオフライン化や起動モード変更は行いません。

## QNAP

QPKGを更新すると、既存の`QDK_DATA_DIR_ICONS="icons"`設定により次の画像が取り込まれます。古いアイコンが残る場合は、QTS管理画面を再読み込みしてください。

| ファイル | サイズ | 用途 |
| --- | --- | --- |
| `qpkg/icons/docExtractor.png` | 64×64 | 標準 |
| `qpkg/icons/docExtractor_80.png` | 80×80 | 大サイズ |
| `qpkg/icons/docExtractor_gray.png` | 64×64 | 停止時グレー |
| `internal/web/static/apple-touch-icon.png` | 180×180 | iPhoneホーム画面 |
| `internal/web/static/favicon.png` | 32×32 | ブラウザータブ |

Web用画像はGoバイナリに埋め込まれ、明示した2つのGETルートだけで配信します。直接アクセスと`/docExtractor`プロキシ経由の両方に対応し、外部画像サービスへの接続やリクエストごとのディスク読み込みは不要です。

## デザイン元

内蔵imagegenで生成。プロンプト: "A polished square app icon for docExtractor, a QNAP document/archive organizer, also used on iPhone home screen. A crisp white document with folded corner rises from a vivid teal folder, with an upward extraction arrow. Dark navy full-bleed background, subtle depth, bold silhouette readable at 64 pixels, generous safe margins. No text, watermark, outer rounded corners, or mockup. Opaque square icon."

生成画像から各サイズへ縮小し、停止時用はグレースケール化しています。CIでPNG形式・寸法、GoテストでURL・ヘッダー・PNGのデコードを検証します。実機での表示確認は別途必要です。
