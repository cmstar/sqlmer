# sqlmer

[![License](https://img.shields.io/badge/license-MIT-brightgreen.svg?style=flat)](https://opensource.org/licenses/MIT)
[![Go](https://github.com/bunnier/sqlmer/actions/workflows/go.yml/badge.svg)](https://github.com/bunnier/sqlmer/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/bunnier/sqlmer)](https://goreportcard.com/report/github.com/bunnier/sqlmer)
[![Go Reference](https://pkg.go.dev/badge/github.com/bunnier/sqlmer.svg)](https://pkg.go.dev/github.com/bunnier/sqlmer)

## 功能简介

数据库访问库，目前支持 MySQL 和 SQL Server。

- SQL语句提供了统一的 `命名参数`、`索引参数` 语法，可直观的使用 map 作为SQL语句参数，并以 map 或 slice 方式返回；
- 提供了面向map的 `参数`、`结果集` 交互接口，事务和非事务访问均可通过相同接口完成；
- 扩展了原生 sql.Rows / sql.Row，使其支持 MapScan 以及 SliceScan；

## 简单样例

```go
func main() {
	// 获取 DbClient，这里使用 SqlServer 做示范，MySQL 也提供了一致的 API 和相应的参数解析逻辑。
	dbClient, err := mssql.NewMsSqlDbClient(
		"server=127.0.0.1:1433; database=test; user id=dev;password=qwer1234;",
		sqlmer.WithConnTimeout(time.Second*15), // 连接超时。
		sqlmer.WithExecTimeout(time.Second*15), // 读写超时(执行语句时候，如果没有指定超时时间，默认用这个)。
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	// 创建测试表。
	_, err = dbClient.Execute(`
CREATE TABLE MainDemo(
	Id INT PRIMARY KEY IDENTITY(1,1) NOT NULL,
	Name VARCHAR(10) NOT NULL,
	Age INT NOT NULL
)`)
	if err != nil {
		log.Fatal(err)
		return
	}

	// 有超时时间的查询。
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	_, err = dbClient.ExecuteContext(ctx, "WAITFOR DELAY '00:00:02'")
	if err != nil {
		fmt.Println("timeout: " + err.Error()) // 预期内的超时~
	}

	// 索引方式插入数据，@p1...@pn，分别对应第 1-n 个参数。
	_, err = dbClient.Execute("INSERT INTO MainDemo(Name, Age) VALUES(@p1, @p2)", "rui", 1)
	if err != nil {
		log.Fatal(err)
		return
	}
	_, err = dbClient.Execute("INSERT INTO MainDemo(Name, Age) VALUES(@p1, @p2)", "bao", 2)
	if err != nil {
		log.Fatal(err)
		return
	}

	// 命名参数查询数据，命名参数采用 map，key 为 sql 语句 @ 之后的参数名，value 为值。
	data, err := dbClient.Get("SELECT * FROM MainDemo WHERE Name=@name", map[string]interface{}{"name": "rui"})
	if err != nil {
		log.Fatal(err)
		return
	}
	fmt.Println(data) // map 方式返回，需要结构的，需要自己转换。

	// 获取第一行第一列，返回的第二个值标示是否为 null，主要用于 nullable 列。
	name, _, err := dbClient.Scalar("SELECT Name FROM MainDemo WHERE Name=@p1", "rui")
	if err != nil {
		log.Fatal(err)
		return
	}
	
	// 返回interface{}，类型可以自己转换。已经统一了 Sql Server 和 MySQL 返回的类型。
	fmt.Println(name.(string)) 

	// 获取增强后的sql.Rows（支持SliceScan、MapScan）。
	sliceRows, err := dbClient.Rows("SELECT Name, GETDATE() FROM MainDemo WHERE Name IN (@p1, @p2)", "rui", "bao")
	if err != nil {
		log.Fatal(err)
		return
	}
	for sliceRows.Next() {
		sliceRow, err := sliceRows.SliceScan() // SliceScan 用 []interface{} 方式返回。
		if err != nil {
			log.Fatal(err)
			return
		}
		fmt.Println(sliceRow...)
	}

	if sliceRows.Err() != nil {
		log.Fatal(err)
		return
	}

	// 删除测试表。
	_, err = dbClient.Execute("DROP TABLE MainDemo")
	if err != nil {
		log.Fatal(err)
		return
	}
}
```

## 类型映射

> nullable 的字段，如果值为 null，默认均以 nil 返回。

### MySql

| DB datatype                                        | Go datatype |
|----------------------------------------------------|-------------|
| varchar / char / text                              | string      |
| tiny int / small int / int / unsigned int / bigint | int64       |
| float / double                                     | float64     |
| decimal                                            | string      |
| date / datetime / timestamp                        | time.Time   |
| bit                                                | []byte      |

### SQL Server

| DB datatype                              | Go datatype |
|------------------------------------------|-------------|
| nvarchar / nchar / varchar / char / text | string      |
| small int / tiny int / int / bigint      | int64       |
| float / real                             | float64     |
| small money / money / decimal            | string      |
| date / datetime / datetime2 / time       | time.Time   |
| binary / varbinary                       | []byte      |
| bit                                      | bool        |

## 测试用例

测试用例 Schema：

1. 编辑 `test_conf.yml` 文件，相应数据库的连接字符串；
2. 如果第 1 步配置的连接字符串有 DDL 权限，可通过调用 `go run ./internal/testcmd/main.go -a PREPARE -c test_conf.yml` 来同时准备 MySQL / SQL Server 环境，如果没有 DDL 权限可自行直接执行 `internal/testenv/*_prepare.sql` 准备环境；
3. 如果第 1 步配置的连接字符串有 DDL 权限，测试结束后可以通过 `go run ./internal/testcmd/main.go -a CLEAN -c test_conf.yml` 销毁测试表，如果没有 DDL 权限可自行直接执行 `internal/testenv/*_clean.sql` 销毁测试表；

另外，如果你和我一样使用 VSCode 作为开发工具，可在配置好 `test_conf.yml` 之后，直接使用 .vscode 中编写好的 Task 来准备环境。
