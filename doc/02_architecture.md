02. アーキテクチャ設計 (Architecture)
ディレクトリ構成
パッケージはすべて internal/ 配下に置き、外部から import できる公開APIにはしない（CLIだけを提供する）。
.
├── main.go                # エントリーポイントとCLIルーティング
├── internal/
│   ├── cmd/               # コマンド定義
│   │   ├── map.go         # ctxmap map コマンド
│   │   ├── deps.go        # ctxmap deps コマンド
│   │   ├── slice.go       # ctxmap slice コマンド
│   │   ├── common.go      # 走査・並列パース・出力の共通処理
│   │   ├── flags.go       # 共通フラグ（--include-tests / --exclude / --changed など）
│   │   ├── git.go         # --changed 用の git 差分取得
│   │   └── tsconfig.go    # tsconfig / jsconfig の paths・baseUrl 解決
│   ├── parser/            # 解析ロジック
│   │   ├── go_parser.go   # go/ast を利用したGo言語パーサー
│   │   └── ts_parser.go   # 正規表現ベースのTS/JSパーサー
│   └── filter/            # ディレクトリ・ファイルのフィルタリング
│       └── ignore.go      # デフォルト除外ルール、.gitignore / .ctxmapignore / --exclude の処理
└── CLAUDE.md              # エージェント用プロンプト
主要コンポーネント
1. CLIルーティング (cmd パッケージ)
標準の flag パッケージ等を利用してサブコマンド（map, deps, slice）をルーティングします。過度なCLIフレームワーク（Cobraなど）はバイナリサイズ削減のために避けるか、最小限の依存にとどめてください。
2. ファイルフィルタリング (filter パッケージ)
対象ディレクトリを filepath.WalkDir などで走査。
規定の除外リスト（.git, node_modules等）にマッチするディレクトリは filepath.SkipDir でスキップ。
拡張子（.go, .ts, .tsx, .js, .jsx）でファイルをフィルタ。
.gitignore のパース処理（簡易実装）を行い、可能ならスキップ処理に統合。
3. Go AST パーサー (internal/parser/go_parser.go)
parser.ParseFile に parser.ParseComments を渡してASTを取得。
ast.Walk で巡回し、FuncDecl, GenDecl (TYPE) のノードを抽出。
トークン浪費を防ぐため、メソッド内の BlockStmt は出力文字列から省くロジックを実装。
4. TS/JS 軽量パーサー (internal/parser/ts_parser.go)
ファイル内容を読み込み、正規表現で以下を抽出。
export (interface|type|class|function|const) ...
import ... from '...'
厳密なAST解析は不要。行ベースまたはブロックベースの正規表現抽出で対応。
