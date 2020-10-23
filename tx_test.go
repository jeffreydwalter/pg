package pg_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-pg/pg/v11"
)

var _ = Describe("Tx", func() {
	var db *pg.DB

	BeforeEach(func() {
		db = pg.Connect(pgOptions())
	})

	AfterEach(func() {
		err := db.Close(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	It("reconnects on bad connection", func() {
		cn, err := db.Pool().Get(context.Background())
		Expect(err).NotTo(HaveOccurred())

		cn.SetNetConn(&badConn{})
		db.Pool().Put(ctx, cn)

		tx, err := db.Begin(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = tx.Rollback(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	It("supports multiple statements", func() {
		tx, err := db.Begin(ctx)
		Expect(err).NotTo(HaveOccurred())

		stmt1, err := tx.Prepare(ctx, `SELECT 'test_multi_prepare_tx1'`)
		Expect(err).NotTo(HaveOccurred())

		stmt2, err := tx.Prepare(ctx, `SELECT 'test_multi_prepare_tx2'`)
		Expect(err).NotTo(HaveOccurred())

		var s1 string
		_, err = stmt1.QueryOne(ctx, pg.Scan(&s1))
		Expect(err).NotTo(HaveOccurred())
		Expect(s1).To(Equal("test_multi_prepare_tx1"))

		var s2 string
		_, err = stmt2.QueryOne(ctx, pg.Scan(&s2))
		Expect(err).NotTo(HaveOccurred())
		Expect(s2).To(Equal("test_multi_prepare_tx2"))

		err = tx.Rollback(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	It("supports CopyFrom and CopyIn", func() {
		data := "hello\t5\nworld\t5\nfoo\t3\nbar\t3\n"

		_, err := db.Exec(ctx, "DROP TABLE IF EXISTS test_copy_from")
		Expect(err).NotTo(HaveOccurred())

		_, err = db.Exec(ctx, "CREATE TABLE test_copy_from(word text, len int)")
		Expect(err).NotTo(HaveOccurred())

		tx1, err := db.Begin(ctx)
		Expect(err).NotTo(HaveOccurred())
		tx2, err := db.Begin(ctx)
		Expect(err).NotTo(HaveOccurred())

		r := strings.NewReader(data)
		res, err := tx1.CopyFrom(ctx, r, "COPY test_copy_from FROM STDIN")
		Expect(err).NotTo(HaveOccurred())
		Expect(res.RowsAffected()).To(Equal(4))

		var count int
		_, err = tx1.QueryOne(ctx, pg.Scan(&count), "SELECT COUNT(*) FROM test_copy_from")
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(4))

		_, err = tx2.QueryOne(ctx, pg.Scan(&count), "SELECT COUNT(*) FROM test_copy_from")
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(0))

		err = tx1.Commit(ctx)
		Expect(err).NotTo(HaveOccurred())

		_, err = tx2.QueryOne(ctx, pg.Scan(&count), "SELECT COUNT(*) FROM test_copy_from")
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(4)) // assuming READ COMMITTED

		err = tx2.Rollback(ctx)
		Expect(err).NotTo(HaveOccurred())

		_, err = db.Exec(ctx, "DROP TABLE IF EXISTS test_copy_from")
		Expect(err).NotTo(HaveOccurred())
	})

	It("supports CopyFrom and CopyIn with errors", func() {
		// too many fields on second line
		data := "hello\t5\nworld\t5\t6\t8\t9\nfoo\t3\nbar\t3\n"

		_, err := db.Exec(ctx, "DROP TABLE IF EXISTS test_copy_from")
		Expect(err).NotTo(HaveOccurred())

		_, err = db.Exec(ctx, "CREATE TABLE test_copy_from(word text, len int)")
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(ctx, "INSERT INTO test_copy_from VALUES ('xxx', 3)")
		Expect(err).NotTo(HaveOccurred())

		tx1, err := db.Begin(ctx)
		Expect(err).NotTo(HaveOccurred())
		tx2, err := db.Begin(ctx)
		Expect(err).NotTo(HaveOccurred())

		_, err = tx1.Exec(ctx, "INSERT INTO test_copy_from VALUES ('yyy', 3)")
		Expect(err).NotTo(HaveOccurred())

		r := strings.NewReader(data)
		_, err = tx1.CopyFrom(ctx, r, "COPY test_copy_from FROM STDIN")
		Expect(err).To(HaveOccurred())

		var count int
		_, err = tx1.QueryOne(ctx, pg.Scan(&count), "SELECT COUNT(*) FROM test_copy_from")
		Expect(err).To(HaveOccurred()) // transaction has errors, cannot proceed

		_, err = tx2.QueryOne(ctx, pg.Scan(&count), "SELECT COUNT(*) FROM test_copy_from")
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1))

		err = tx1.Commit(ctx)
		Expect(err).NotTo(HaveOccurred()) // actually ROLLBACK happens here

		_, err = tx2.QueryOne(ctx, pg.Scan(&count), "SELECT COUNT(*) FROM test_copy_from")
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1)) // other transaction was rolled back so it's not 2 and not 6

		err = tx2.Rollback(ctx)
		Expect(err).NotTo(HaveOccurred())

		_, err = db.Exec(ctx, "DROP TABLE IF EXISTS test_copy_from")
		Expect(err).NotTo(HaveOccurred())
	})

	It("drops bad connections", func() {
		_ = db.RunInTransaction(ctx, func(ctx context.Context, tx *pg.Tx) error {
			stmt, err := tx.Prepare(ctx, "invalid statement")
			if err != nil {
				return err
			}
			return stmt.Close(ctx)
		})

		_, err := db.Exec(ctx, "select 1")
		Expect(err).NotTo(HaveOccurred())
	})
})
