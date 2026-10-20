# Aster — 独自プログラミング言語の実行可能プロトタイプ

Asterは、静的型検査を持つ独自言語と、複数の実行方式を統合する設計を検証するためのプロジェクトです。レキサー、パーサー、AST、型検査器、バイトコードコンパイラー、仮想マシン、ネイティブ実行ファイル生成、CLI、テスト、仕様書を含みます。

> **重要：これは初期段階の実行可能なプロトタイプです。** Web、AI・データ分析、業務システム、OS・組み込み、ゲーム、スマートフォンの全分野を実用水準でカバーする完成版ではありません。Rust、C/C++、Python、TypeScript、Java、Swift、Kotlinなどを置き換えられる品質だとは主張しません。未実装の機能は以下のとおり明示しています。

## 目指す設計

- 静的型付けを基本とし、ローカル変数は型推論で記述量を減らす。
- デフォルト不変の変数と明示的な可変宣言を採用する。現行のエラー処理は実行時検査を中心とし、`Result` 型などの明示的なエラー型は未実装。
- 共通の言語仕様から、VM実行とネイティブ実行の双方を提供する。
- VMとネイティブ側で整数オーバーフローなどの演算規則を揃える。
- 標準言語機能と、将来の分野別ライブラリを明確に分ける。

## 実装済みの機能（v0.1）

| 機能 | 状況 |
|---|---|
| レキサー、位置情報、入れ子のブロックコメント | 実装済み |
| 再帰下降パーサーとPratt式パーサー | 実装済み |
| ソースフォーマッター | 実装済み（コメントと文字列リテラルの原表記を保持） |
| `i64`、`f64`、`bool`、`string`、`unit` の型検査 | 実装済み |
| 明示的数値変換（`i64(value)`、`f64(value)`） | 実装済み（範囲検査とVM／Native一致テストを含む） |
| 型推論、関数、再帰、条件分岐、`while`、`return` | 実装済み |
| 不変バインディング、`let mut`、配列 `Array<T>` | 実装済み |
| 配列インデックス・範囲チェック・`len`・スライス | 実装済み（入れ子の要素代入と独立したスライス値に対応） |
| バージョン付きJSONバイトコード、構造・スタック高・型・ローカル初期化検証、VM | 実装済み |
| ネイティブ実行ファイル生成 | AsterコードをGoコードへ変換し、Goコンパイラーを利用 |
| WebAssembly、LLVM直接生成、GPU、AIランタイム連携 | 未実装 |
| `struct`、`enum`、`Option`、`Result`、モジュール、パッケージ管理 | 未実装 |
| `async/await`、クロージャー、マクロ、完全な所有権・借用検査 | 未実装 |
| 独立したセキュリティ監査・本番用途の性能保証 | 未実施 |

## 必要な環境

- Go 1.23以降。ネイティブバックエンドでは `go` と `gofmt` が `PATH` 上に必要です（`gofmt` は通常Goに同梱されています）。
- 現在の実装はサードパーティのGoモジュールを必要としません。

## ビルドとテスト

リポジトリのルートで実行します。

```bash
go test ./...
go vet ./...
mkdir -p ./bin
go build -o ./bin/aster ./cmd/aster
```

ビルドしたCLIは `bin/aster` です。Windowsでは `aster.exe` などの拡張子を使用してください。

## 使い方

### ソースをVMで実行

```bash
./bin/aster run examples/hello.ast
./bin/aster run examples/arrays.ast
./bin/aster run examples/fibonacci.ast
```

### 型検査だけ実行

```bash
./bin/aster check examples/fibonacci.ast
```

### ソースの整形

```bash
./bin/aster fmt examples/hello.ast
./bin/aster fmt --check examples/hello.ast
```

aster fmt は構文を検証してから整形し、コメントと文字列リテラルのエスケープ表記を保持します。--check はファイルを変更せず、整形済みかどうかを検証します。

### バイトコードへコンパイルして再実行

```bash
./bin/aster compile -o fibonacci.abc.json examples/fibonacci.ast
./bin/aster exec fibonacci.abc.json
```

### ネイティブ実行ファイルを生成

```bash
./bin/aster build -o ./fibonacci examples/fibonacci.ast
./fibonacci
```

VMとネイティブの実行経路を直接切り替えることもできます。

```bash
./bin/aster run --engine vm examples/arrays.ast
./bin/aster run --engine native examples/arrays.ast
```

ネイティブバックエンドは、型検査器が生成した型付きIRからGoコードを生成し、ホスト側のGoコンパイラーを呼び出します。Aster独自の機械語コード生成器やLLVMバックエンドはまだないため、ネイティブ実行ファイルのビルドにはGoのツールチェーンが必要です。呼び出し深度、文字列サイズ、配列サイズ、出力サイズの上限はVMとNativeの双方に適用されます。一方、VM固有の命令数、オペランドスタック、アクティブローカル枠の制限はNativeには適用されません。ネイティブ単体ビルドの既定期限は2分、`aster run --engine native` のビルドと実行を合わせた既定期限は5分です。ネイティブ実行は呼び出し元と同じOS権限で動作し、サンドボックスではありません。

## Asterのコード例

```aster
fn sum(values: Array<i64>) -> i64 {
    let mut index: i64 = 0;
    let mut total: i64 = 0;
    while index < len(values) {
        total = total + values[index];
        index = index + 1;
    }
    return total;
}

let values: Array<i64> = [3, 5, 8, 13, 21];
print(values);
print(sum(values));
```

この例では、`Array<i64>` は64ビット整数を要素に持つ動的配列です。型検査は配列の要素型を確認し、VMでは範囲外インデックスを実行時エラーにします。変更可能なバインディングでは `values[0] = 9;` のように要素を書き換えられ、`matrix[row][column] = value;` のような入れ子の代入も値セマンティクスを維持します。

### 対応型と組み込み関数

- `i64`: 符号付き64ビット整数。オーバーフローは実行時エラー。
- `f64`: 64ビット浮動小数点数。現在の仕様ではゼロ除算と非有限の演算結果をエラーにする。
- `bool`: 真偽値。
- `string`: UTF-8文字列。`len` はUnicodeコードポイント数を返す（書記素クラスタ数ではない）。
- `unit`: 戻り値のない関数の型。変数には保存できない。
- `Array<T>`: 同じ型の値を格納する動的配列。`let mut` の要素代入と入れ子のインデックス代入に対応。`values[start:end]` の半開区間スライスは独立した配列値を返し、空スライスも許可します。空配列リテラルは未対応。

組み込み関数は `print(value);`、`len(string_or_array)`、`assert(condition);` です。数値型間の暗黙変換はありません。明示変換には `i64(value)` と `f64(value)` を使います。`f64(i64値)` はIEEE 754のbinary64変換なので整数精度を失う場合があり、`i64(f64値)` は小数部をゼロ方向に切り捨て、`[-2^63, 2^63)` の範囲外、NaN、Infinityを実行時エラーにします。

## ディレクトリ構成

```text
cmd/aster/             CLI
internal/ast/          AST
internal/lexer/        字句解析と字句テスト・ファズテスト
internal/parser/       構文解析とファズテスト
internal/checker/      静的型検査と型付きIRの生成
internal/ir/           共通の型付き中間表現
internal/bytecode/     バイトコード形式・検証・IRコンパイラー
internal/vm/           仮想マシン・テスト・ベンチマーク
internal/codegen/      ネイティブ向けGoコード生成
internal/toolchain/    Goツールチェーン呼び出し
docs/                  仕様・構成・セキュリティ・ロードマップ・ADR
examples/              動作するAsterプログラム
bench/                 ベンチマーク方法
stdlib/                将来の標準ライブラリの境界
tests/                 バックエンド間の統合テスト
```

## テストとベンチマーク

```bash
make check
make examples
go test ./internal/vm -run '^$' -bench '^BenchmarkVM' -benchmem -count=5
```

テストには、構文解析・型エラー・配列境界チェック・整数オーバーフロー・再帰・論理演算の短絡評価・バイトコードの保存と読込・VMとネイティブ実行の出力一致が含まれます。ベンチマークは測定用の入口であり、現在のREADMEは他言語との速度優位性を主張していません。

## 品質検証とCI

ローカルで以下を実行すると、書式チェック、テスト、静的解析、ビルドをまとめて確認できます。

```bash
make check
make examples
go test -race ./...
```

GitHub Actions（[`.github/workflows/ci.yml`](.github/workflows/ci.yml)）でも、Go 1.23と最新安定版Goについて、書式チェック、テスト、race detector、静的解析、ビルド、サンプル実行を検証します。

## 詳細ドキュメント

- [`docs/language-spec.md`](docs/language-spec.md): 現在実装されている文法と意味
- [`docs/architecture.md`](docs/architecture.md): 処理系の構成
- [`docs/diagnostics.md`](docs/diagnostics.md): 字句解析・構文解析・型検査の安定した診断コード
- [`docs/security-model.md`](docs/security-model.md): 安全性の設計と未解決事項
- [`docs/roadmap.md`](docs/roadmap.md): 段階的な開発計画
- [`docs/contributing.md`](docs/contributing.md): 開発ルール
- [`bench/README.md`](bench/README.md): ベンチマーク方法
- [`CHANGELOG.md`](CHANGELOG.md): 変更履歴

MITライセンス。詳細は [`LICENSE`](LICENSE) を参照してください。
