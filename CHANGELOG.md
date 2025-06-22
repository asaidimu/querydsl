# [5.0.0](https://github.com/asaidimu/querydsl/compare/v4.0.0...v5.0.0) (2025-06-22)


* refactor(core)!: streamline query execution interfaces and add CRUD ([2629f73](https://github.com/asaidimu/querydsl/commit/2629f73d750d0854d0b7fe1afa9e97570842ef4f))


### BREAKING CHANGES

* The 'tableName' parameter has been removed from QueryExecutor.Query, QueryExecutor.Update, QueryGenerator.GenerateSelectSQL, and QueryGenerator.GenerateUpdateSQL. Existing calls to these methods must be updated, and table context should now be managed by the executor/generator instance.
Additionally, the concrete SQLite implementations (pkg/sqlite/executor.go and pkg/sqlite/query.go) have been removed, requiring users to update their SQLite integration or provide new implementations conforming to the updated interfaces.

# [4.0.0](https://github.com/asaidimu/querydsl/compare/v3.0.0...v4.0.0) (2025-06-20)


* refactor(executor)!: change Update method return type to rows affected ([f050edc](https://github.com/asaidimu/querydsl/commit/f050edc24220a8e7dfdcbc6fef0b559409bb05f3))


### BREAKING CHANGES

* The QueryExecutor.Update method's signature has changed
from returning sql.Result to int64. Consumers of this interface or the
SqliteExecutor.Update method must update their code to expect an int64
representing the number of rows affected.

# [3.0.0](https://github.com/asaidimu/querydsl/compare/v2.0.0...v3.0.0) (2025-06-20)


* feat(executor)!: Add database update support and rename query methods ([737149b](https://github.com/asaidimu/querydsl/commit/737149b688b98e67c04f49178aa16467847ea5e4))


### BREAKING CHANGES

* The QueryExecutor.Execute method has been renamed to Query.
The QueryGenerator.Generate method has been renamed to GenerateSelectSQL.
Update calls to these methods in your code.

# [2.0.0](https://github.com/asaidimu/querydsl/compare/v1.0.0...v2.0.0) (2025-06-20)


* refactor(query-generation)!: Make SqliteExecutor's query generation pluggable ([d4fc2c7](https://github.com/asaidimu/querydsl/commit/d4fc2c7fa41c343a7b0684ddfe431edce91f6811))


### BREAKING CHANGES

* The 'NewSqliteExecutor' constructor now requires a
'querydsl.QueryGenerator' argument. Existing calls like
'sqlite.NewSqliteExecutor(db)' must be updated to
'sqlite.NewSqliteExecutor(db, sqlite.NewSqliteQuery())' or by providing a
custom QueryGenerator implementation.

# 1.0.0 (2025-06-18)


* feat(project)!: introduce declarative Query DSL with hybrid execution ([723f3bb](https://github.com/asaidimu/querydsl/commit/723f3bb506837a0fe0d3b8093c43066952441e7d))


### BREAKING CHANGES

* The project is no longer a simple "Hello, World!" executable. The `main.go` and `pkg/greeting.go` files, and their associated test, have been removed. This project is now a library intended for integration into other Go applications, and its previous trivial functionality is entirely gone.
