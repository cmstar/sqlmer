package mysql

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bunnier/sqlmer"
	"github.com/bunnier/sqlmer/internal/testenv"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// 初始化测试配置。
var testConf testenv.TestConf = testenv.MustLoadTestConfig("../test_conf.yml")

// 用于获取一个 MySql 测试库的 DbClient 对象。
func getMySqlDbClient() (sqlmer.DbClient, error) {
	return NewMySqlDbClient(
		testConf.MySql,
		sqlmer.WithConnTimeout(time.Second*15),
		sqlmer.WithExecTimeout(time.Second*15),
	)
}

func Test_NewMySqlDbClient(t *testing.T) {
	dbClient, err := getMySqlDbClient()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(dbClient.ConnectionString(), testConf.MySql) {
		t.Errorf("mysqlDbClient.ConnectionString() connString = %v, want contains  %v", dbClient.ConnectionString(), testConf.MySql)
	}

	_, err = NewMySqlDbClient("test",
		sqlmer.WithConnTimeout(time.Second*15),
		sqlmer.WithExecTimeout(time.Second*15),
		sqlmer.WithPingCheck(true))
	if err == nil {
		t.Errorf("mysqlDbClient.NewMsSqlDbClient() err = nil, want has a err")
	}
}

func Test_bindMySqlArgs(t *testing.T) {
	testCases := []struct {
		name      string
		oriSql    string
		args      []interface{}
		wantSql   string
		wantParam []interface{}
		wantErr   error
	}{
		{
			"map1",
			"SELECT * FROM go_TypeTest WHERE id=@id",
			[]interface{}{
				map[string]interface{}{
					"id": 1,
				},
			},
			"SELECT * FROM go_TypeTest WHERE id=?",
			[]interface{}{1},
			nil,
		},
		{
			"map2",
			"SELECT * FROM go_TypeTest WHERE idv2=@id_id",
			[]interface{}{
				map[string]interface{}{
					"id_id": 1,
				},
			},
			"SELECT * FROM go_TypeTest WHERE idv2=?",
			[]interface{}{1},
			nil,
		},
		{
			"map3",
			"SELECT * FROM go_TypeTest WHERE idv2=@id_id AND id=@id",
			[]interface{}{
				map[string]interface{}{
					"id_id": 1,
					"id":    2,
				},
			},
			"SELECT * FROM go_TypeTest WHERE idv2=? AND id=?",
			[]interface{}{1, 2},
			nil,
		},
		{
			"map_name_err",
			"SELECT * FROM go_TypeTest WHERE id=@id1 OR id=@id2",
			[]interface{}{
				map[string]interface{}{
					"id": 1,
				},
			},
			"",
			nil,
			sqlmer.ErrSql,
		},
		{
			"index",
			"SELECT * FROM go_TypeTest WHERE id=@p1",
			[]interface{}{1},
			"SELECT * FROM go_TypeTest WHERE id=?",
			[]interface{}{1},
			nil,
		},
		{
			"index_index_err1",
			"SELECT * FROM go_TypeTest WHERE id=@p1 AND id=@p2",
			[]interface{}{1},
			"",
			nil,
			sqlmer.ErrSql,
		},
		{
			"index_index_err2",
			"SELECT * FROM go_TypeTest WHERE id=@p3",
			[]interface{}{1},
			"",
			nil,
			sqlmer.ErrSql,
		},
		{
			"index_index_err3",
			"SELECT * FROM go_TypeTest WHERE id=@test",
			[]interface{}{1},
			"",
			nil,
			sqlmer.ErrSql,
		},
		{
			"index_index_err4",
			"SELECT * FROM go_TypeTest WHERE id=@pttt",
			[]interface{}{1},
			"",
			nil,
			sqlmer.ErrSql,
		},
		{
			"index_reuse_index",
			"SELECT * FROM go_TypeTest WHERE id=@p1 AND id=@p1",
			[]interface{}{1},
			"SELECT * FROM go_TypeTest WHERE id=? AND id=?",
			[]interface{}{1, 1},
			nil,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fixedSql, args, err := bindMySqlArgs(tt.oriSql, tt.args...)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("mysqlDbClient.bindMsSqlArgs() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
			} else {
				if err != nil {
					t.Error(err)
					return
				}
				if fixedSql != tt.wantSql {
					t.Errorf("mysqlDbClient.bindMsSqlArgs() sql = %v, wantSql %v", fixedSql, tt.wantSql)
				}

				if !reflect.DeepEqual(args, tt.wantParam) {
					t.Errorf("mysqlDbClient.bindMsSqlArgs() args = %v, wantParam %v", args, tt.wantParam)
				}
			}
		})
	}
}

func Test_parseMySqlNamedSql(t *testing.T) {
	testSqls := map[string][]string{
		"SELECT * FROM go_TypeTest WHERE @@id=1":                                  {"SELECT * FROM go_TypeTest WHERE @id=1", ""},
		"SELECT * FROM go_TypeTest WHERE id=@id":                                  {"SELECT * FROM go_TypeTest WHERE id=?", "id"},
		"SELECT * FROM go_TypeTest WHERE id=@id AND id=@id":                       {"SELECT * FROM go_TypeTest WHERE id=? AND id=?", "id,id"},
		"SELECT * FROM go_TypeTest WHERE id=@id AND varcharTest='@varcharTest'":   {"SELECT * FROM go_TypeTest WHERE id=? AND varcharTest='@varcharTest'", "id"},
		"SELECT * FROM go_TypeTest WHERE id=@id AND varcharTest=@varcharTest":     {"SELECT * FROM go_TypeTest WHERE id=? AND varcharTest=?", "id,varcharTest"},
		"SELECT * FROM go_TypeTest WHERE varcharTest=@varcharTest AND id=@id":     {"SELECT * FROM go_TypeTest WHERE varcharTest=? AND id=?", "varcharTest,id"},
		"SELECT * FROM go_TypeTest WHERE varcharTest='@varcharTest' AND id='@id'": {"SELECT * FROM go_TypeTest WHERE varcharTest='@varcharTest' AND id='@id'", ""},
	}

	var errGroup errgroup.Group
	for i := 0; i < 10; i++ { // 这边测试下并发，开10个goroutine并行测试。
		errGroup.Go(func() error {
			for inputSql, expected := range testSqls {
				namedParsedResult := parseMySqlNamedSql(inputSql)
				if namedParsedResult.Sql != expected[0] || strings.Join(namedParsedResult.Names, ",") != expected[1] {
					return fmt.Errorf("expected sql=%s, param=%s\nActual sql=%s, param=%s",
						expected[0], expected[1],
						namedParsedResult.Sql, strings.Join(namedParsedResult.Names, ","))
				}
			}
			return nil
		})
	}

	if err := errGroup.Wait(); err != nil {
		t.Errorf("mysqlDbClient.parseMySqlNamedSql() err = %v, wantErr = nil", err)
		return
	}
}

func Test_internalDbClient_Scalar(t *testing.T) {
	mysqlClient, err := getMySqlDbClient()
	if err != nil {
		t.Fatal(err)
	}
	type args struct {
		sqlText string
		args    []interface{}
	}
	tests := []struct {
		name    string
		client  sqlmer.DbClient
		args    args
		want    interface{}
		wantErr bool
	}{
		{
			"mysql",
			mysqlClient,
			args{
				"SELECT Id FROM go_TypeTest WHERE id=@p1",
				[]interface{}{1},
			},
			int64(1),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := tt.client.Scalar(tt.args.sqlText, tt.args.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("internalDbClient.Scalar() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("internalDbClient.Scalar() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_internalDbClient_Execute(t *testing.T) {
	now := time.Now()
	mysqlClient, err := getMySqlDbClient()
	if err != nil {
		t.Fatal(err)
	}
	type args struct {
		sqlText string
		args    []interface{}
	}
	tests := []struct {
		name    string
		client  sqlmer.DbClient
		args    args
		wantErr bool
	}{
		{
			"mysql",
			mysqlClient,
			args{
				`INSERT INTO go_TypeTest(varcharTest, charTest, charTextTest, dateTest, dateTimeTest, timestampTest, floatTest, doubleTest, decimalTest, bitTest)
				VALUES (N'行5', '行5char', '行5text','2021-07-05','2021-07-05 15:38:50.425','2021-07-05 15:38:50.425', 5.456, 5.15678, 5.45678999, 1);`,
				[]interface{}{now},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effectRow, err := tt.client.Execute(tt.args.sqlText, tt.args.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("internalDbClient.Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			if effectRow != int64(1) {
				if (err != nil) != tt.wantErr {
					t.Errorf("internalDbClient.Execute() error = %v, wantErr %v", err, tt.wantErr)
				}
			}

			err = tt.client.SizedExecute(1, tt.args.sqlText, tt.args.args...)
			if err != nil {
				t.Errorf("internalDbClient.SizedExecute() error = %v, wantErr %v", err, tt.wantErr)
			}

			err = tt.client.SizedExecute(2, tt.args.sqlText, tt.args.args...)
			if !errors.Is(err, sqlmer.ErrSql) {
				t.Errorf("internalDbClient.SizedExecute() error = %v, wantErr DbSqlError", err)
			}
		})
	}
}

func Test_internalDbClient_Exists(t *testing.T) {
	mysqlClient, err := getMySqlDbClient()
	if err != nil {
		t.Fatal(err)
	}
	type args struct {
		sqlText string
		args    []interface{}
	}
	tests := []struct {
		name    string
		client  sqlmer.DbClient
		args    args
		want    bool
		wantErr bool
	}{
		{
			"mysql_exist",
			mysqlClient,
			args{
				"SELECT varcharTest,dateTest,dateTimeTest,timestampTest,decimalTest FROM go_TypeTest WHERE id=1",
				[]interface{}{},
			},
			true,
			false,
		},
		{
			"mysql_notexist",
			mysqlClient,
			args{
				"SELECT varcharTest,dateTest,dateTimeTest,timestampTest,decimalTest FROM go_TypeTest WHERE id=10000",
				[]interface{}{},
			},
			false,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.client.Exists(tt.args.sqlText, tt.args.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("internalDbClient.Exists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("internalDbClient.Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_internalDbClient_Get(t *testing.T) {
	mysqlClient, err := getMySqlDbClient()
	if err != nil {
		t.Fatal(err)
	}
	type args struct {
		sqlText string
		args    []interface{}
	}
	tests := []struct {
		name    string
		client  sqlmer.DbClient
		args    args
		want    map[string]interface{}
		wantErr bool
	}{
		{
			"mysql_nullable_null",
			mysqlClient,
			args{
				`SELECT varcharTest, charTest, charTextTest, dateTest, dateTimeTest, timestampTest, floatTest, doubleTest, decimalTest, bitTest,
				nullVarcharTest, nullCharTest, nullTextTest, nullDateTest, nullDateTimeTest, nullTimestampTest, nullFloatTest, nullDoubleTest, nullDecimalTest, nullBitTest 
				FROM go_TypeTest WHERE id=1`,
				[]interface{}{},
			},
			map[string]interface{}{
				"varcharTest":       "行1",
				"charTest":          "行1char",
				"charTextTest":      "行1text",
				"dateTest":          time.Date(2021, 7, 1, 0, 0, 0, 0, time.UTC),
				"dateTimeTest":      time.Date(2021, 7, 1, 15, 38, 50, 0, time.UTC),
				"timestampTest":     time.Date(2021, 7, 1, 15, 38, 50, 0, time.UTC),
				"floatTest":         float64(1.456),
				"doubleTest":        float64(1.15678),
				"decimalTest":       "1.4567899900",
				"bitTest":           sql.RawBytes([]uint8{1}),
				"nullVarcharTest":   nil,
				"nullCharTest":      nil,
				"nullTextTest":      nil,
				"nullDateTest":      nil,
				"nullDateTimeTest":  nil,
				"nullTimestampTest": nil,
				"nullFloatTest":     nil,
				"nullDoubleTest":    nil,
				"nullDecimalTest":   nil,
				"nullBitTest":       nil,
			},
			false,
		},
		{
			"mysql_nullable_not_null",
			mysqlClient,
			args{
				`SELECT varcharTest, charTest, charTextTest, dateTest, dateTimeTest, timestampTest, floatTest, doubleTest, decimalTest, bitTest,
				nullVarcharTest, nullCharTest, nullTextTest, nullDateTest, nullDateTimeTest, nullTimestampTest, nullFloatTest, nullDoubleTest, nullDecimalTest, nullBitTest
				FROM go_TypeTest WHERE id=3`,
				[]interface{}{},
			},
			map[string]interface{}{
				"varcharTest":       "行3",
				"charTest":          "行3char",
				"charTextTest":      "行3text",
				"dateTest":          time.Date(2021, 7, 3, 0, 0, 0, 0, time.UTC),
				"dateTimeTest":      time.Date(2021, 7, 3, 15, 38, 50, 0, time.UTC),
				"timestampTest":     time.Date(2021, 7, 3, 15, 38, 50, 0, time.UTC),
				"floatTest":         float64(3.456),
				"doubleTest":        float64(3.15678),
				"decimalTest":       "3.4567899900",
				"bitTest":           sql.RawBytes([]uint8{1}),
				"nullVarcharTest":   "行3",
				"nullCharTest":      "行3char",
				"nullTextTest":      "行3text",
				"nullDateTest":      time.Date(2021, 7, 3, 0, 0, 0, 0, time.UTC),
				"nullDateTimeTest":  time.Date(2021, 7, 3, 15, 38, 50, 0, time.UTC),
				"nullTimestampTest": time.Date(2021, 7, 3, 15, 38, 50, 0, time.UTC),
				"nullFloatTest":     float64(3.456), // 注意，这里有点特殊，因为 nullable 类型只有 float64，所以转换后 nullable 的 float 不会是 float32
				"nullDoubleTest":    float64(3.15678),
				"nullDecimalTest":   "3.4567899900",
				"nullBitTest":       sql.RawBytes([]uint8{1}),
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.client.Get(tt.args.sqlText, tt.args.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("internalDbClient.Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			for k, v := range got {
				wantV := tt.want[k]
				if !reflect.DeepEqual(v, wantV) {
					t.Errorf("fieldname = %s, internalDbClient.Get() = %v, want %v", k, v, wantV)
				}
			}
		})
	}
}

func Test_internalDbClient_SliceGet(t *testing.T) {
	mysqlClient, err := getMySqlDbClient()
	if err != nil {
		t.Fatal(err)
	}
	type args struct {
		sqlText string
		args    []interface{}
	}
	tests := []struct {
		name    string
		client  sqlmer.DbClient
		args    args
		want    []map[string]interface{}
		wantErr bool
	}{
		{
			"mysql",
			mysqlClient,
			args{
				"SELECT varcharTest,dateTest,dateTimeTest,timestampTest,decimalTest FROM go_TypeTest WHERE id IN (1,2)",
				[]interface{}{},
			},
			[]map[string]interface{}{
				{
					"varcharTest":   "行1",
					"dateTest":      time.Date(2021, 7, 1, 0, 0, 0, 0, time.UTC),
					"dateTimeTest":  time.Date(2021, 7, 1, 15, 38, 50, 0, time.UTC),
					"timestampTest": time.Date(2021, 7, 1, 15, 38, 50, 0, time.UTC),
					"decimalTest":   "1.4567899900",
				},
				{
					"varcharTest":   "行2",
					"dateTest":      time.Date(2021, 7, 2, 0, 0, 0, 0, time.UTC),
					"dateTimeTest":  time.Date(2021, 7, 2, 15, 38, 50, 0, time.UTC),
					"timestampTest": time.Date(2021, 7, 2, 15, 38, 50, 0, time.UTC),
					"decimalTest":   "2.4567899900",
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.client.SliceGet(tt.args.sqlText, tt.args.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("internalDbClient.Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("internalDbClient.Get() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_internalDbClient_Rows(t *testing.T) {
	mysqlClient, err := getMySqlDbClient()
	if err != nil {
		t.Fatal(err)
	}
	type args struct {
		sqlText string
		args    []interface{}
	}
	tests := []struct {
		name    string
		client  sqlmer.DbClient
		args    args
		want    []map[string]interface{}
		wantErr bool
	}{
		{
			"mysql",
			mysqlClient,
			args{
				"SELECT varcharTest,dateTest,dateTimeTest,timestampTest,decimalTest FROM go_TypeTest WHERE id IN (1,2)",
				[]interface{}{},
			},
			[]map[string]interface{}{
				{
					"varcharTest":   "行1",
					"dateTest":      time.Date(2021, 7, 1, 0, 0, 0, 0, time.UTC),
					"dateTimeTest":  time.Date(2021, 7, 1, 15, 38, 50, 0, time.UTC),
					"timestampTest": time.Date(2021, 7, 1, 15, 38, 50, 0, time.UTC),
					"decimalTest":   "1.4567899900",
				},
				{
					"varcharTest":   "行2",
					"dateTest":      time.Date(2021, 7, 2, 0, 0, 0, 0, time.UTC),
					"dateTimeTest":  time.Date(2021, 7, 2, 15, 38, 50, 0, time.UTC),
					"timestampTest": time.Date(2021, 7, 2, 15, 38, 50, 0, time.UTC),
					"decimalTest":   "2.4567899900",
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := tt.client.Rows(tt.args.sqlText, tt.args.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("internalDbClient.Rows() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			defer rows.Close()
			index := 0
			for rows.Next() {
				got := make(map[string]interface{})
				err := rows.MapScan(got)
				if (err != nil) != tt.wantErr {
					t.Errorf("internalDbClient.Rows() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !reflect.DeepEqual(got, tt.want[index]) {
					t.Errorf("internalDbClient.Get() = %v, want %v", got, tt.want)
					return
				}
				index++
			}
		})
	}
}
