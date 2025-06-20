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
