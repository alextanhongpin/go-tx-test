package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

// 2019/02/28 22:13:48 [john]: start update
// 2019/02/28 22:13:48 [john]: before sleep
// 2019/02/28 22:13:49 [jane]: start intercept
// 2019/02/28 22:13:49 [john]: start intercept
// 2019/02/28 22:13:49 [jane]: updated, 1 rows affected
// 2019/02/28 22:13:49 [jane]: querying data
// 2019/02/28 22:13:49 [jane]: completed. name=something else and is_set=false
// 2019/02/28 22:13:53 [john]: after sleep
// 2019/02/28 22:13:53 [john]: updated, 0 rows affected
// 2019/02/28 22:13:53 [john]: querying data
// 2019/02/28 22:13:53 [john]: completed. name=john and is_set=true
// 2019/02/28 22:13:53 terminating

func main() {

	connStr := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASS"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_NAME"),
	)
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS test(
		id INT AUTO_INCREMENT,
		name VARCHAR(255) NOT NULL DEFAULT '',
		is_set BOOL NOT NULL DEFAULT 0,
		PRIMARY KEY (id)
	)
	`)
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(`
	TRUNCATE TABLE test
	`)

	johnID, err := insert(db, "john")
	if err != nil {
		log.Println(err)
	}

	janeID, err := insert(db, "jane")
	if err != nil {
		log.Println(err)
	}

	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		defer wg.Done()
		txUpdate(johnID, db, "john")
	}()
	go func() {
		defer wg.Done()
		time.Sleep(1 * time.Second)
		// The transaction above is setting the is_set = 1,
		// and hence this query will never be successful,
		// because the row has been locked.
		interceptRow(johnID, db, "john")
	}()
	go func() {
		defer wg.Done()
		time.Sleep(1 * time.Second)
		// The row jane however is not locked, so the update
		// can still proceed.
		interceptRow(janeID, db, "jane")
	}()
	wg.Wait()
	log.Println("terminating")
}

func insert(db *sql.DB, name string) (int64, error) {
	res, err := db.Exec(`
	INSERT INTO test (name) VALUES (?)
	`, name)

	if err != nil {
		return -1, err
	}
	id, err := res.LastInsertId()
	return id, err
}

func txUpdate(id int64, db *sql.DB, name string) error {
	ns := fmt.Sprintf("[%s]", name)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	log.Printf("%s: start update\n", ns)
	_, err = tx.Exec("UPDATE test SET is_set = 1 WHERE id = ?", id)
	if err != nil {
		return errors.Wrap(tx.Rollback(), err.Error())
	}
	log.Printf("%s: before sleep\n", ns)
	// Simulate transaction locking... (another statement will wait for
	// this to complete)
	time.Sleep(5 * time.Second)
	log.Printf("%s: after sleep\n", ns)
	return tx.Commit()
}

func interceptRow(id int64, db *sql.DB, name string) error {
	ns := fmt.Sprintf("[%s]", name)
	log.Printf("%s: start intercept", ns)
	res, err := db.Exec("UPDATE test SET name = 'something else' WHERE id = ? AND is_set = 0", id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	log.Printf("%s: updated, %d rows affected \n", ns, rows)
	var s string
	var b bool
	log.Printf("%s: querying data\n", ns)
	err = db.QueryRow("SELECT name, is_set FROM test WHERE id = ?", id).Scan(&s, &b)
	log.Printf("%s: completed. name=%s and is_set=%t\n", ns, s, b)
	return err
}
