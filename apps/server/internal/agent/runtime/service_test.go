package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

func TestNewServiceRejectsNilPool(t *testing.T) {
	_, err := NewService(nil, db.New(fakeDBTX{}))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestNewServiceRejectsNilQueries(t *testing.T) {
	_, err := NewService(&fakeBeginner{}, nil)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
}

func TestAppendMessageRejectsMissingThread(t *testing.T) {
	svc, err := NewService(&fakeBeginner{}, db.New(fakeDBTX{}))
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.AppendMessage(context.Background(), AppendMessageParams{
		WorkspaceID: uuidWithByte(1),
		Role:        "user",
		Content:     []byte(`{"text":"hello"}`),
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestAppendMessageDefaultsTextMessageType(t *testing.T) {
	beginner := &fakeBeginner{}
	svc, err := NewService(beginner, db.New(fakeDBTX{}))
	if err != nil {
		t.Fatal(err)
	}

	threadID := uuidWithByte(2)
	msg, err := svc.AppendMessage(context.Background(), AppendMessageParams{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    threadID,
		Role:        "user",
		Content:     []byte(`{"text":"hello"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	if msg.Seq != 1 {
		t.Fatalf("seq = %d, want 1", msg.Seq)
	}
	if msg.MessageType != "text" {
		t.Fatalf("message type = %q, want text", msg.MessageType)
	}
	if !beginner.tx.committed {
		t.Fatal("expected append message to commit transaction")
	}
	if beginner.tx.rolledBack {
		t.Fatal("append message must not roll back after successful commit")
	}
	if got := beginner.tx.createdMessageType; got != "text" {
		t.Fatalf("created message type = %q, want text", got)
	}
	if beginner.tx.nextSeqThreadID != threadID {
		t.Fatalf("next seq thread id = %v, want %v", beginner.tx.nextSeqThreadID, threadID)
	}
}

func TestCreateTaskRejectsInvalidAttempts(t *testing.T) {
	svc, err := NewService(&fakeBeginner{}, db.New(fakeDBTX{}))
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.CreateTask(context.Background(), CreateTaskParams{
		WorkspaceID: uuidWithByte(1),
		Role:        "producer",
		TaskType:    "producer_turn",
		MaxAttempts: 0,
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestCreateTaskAllowsScopedAgentTaskTypes(t *testing.T) {
	svc, err := NewService(&fakeBeginner{}, db.New(fakeDBTX{}))
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := uuidWithByte(1)
	threadID := uuidWithByte(2)
	shotID := uuidWithByte(3)

	cases := []struct {
		role     string
		taskType string
	}{
		{role: "craftsman", taskType: "craftsman_turn"},
		{role: "worker", taskType: "worker_generation"},
		{role: "reviewer", taskType: "reviewer_turn"},
		{role: "system", taskType: "dependency_scheduler"},
	}
	for _, tc := range cases {
		task, err := svc.CreateTask(context.Background(), CreateTaskParams{
			WorkspaceID: workspaceID,
			ThreadID:    threadID,
			Role:        tc.role,
			ScopeType:   "shot",
			ScopeID:     shotID,
			TaskType:    tc.taskType,
			MaxAttempts: 3,
		})
		if err != nil {
			t.Fatalf("%s rejected: %v", tc.taskType, err)
		}
		if task.TaskType != tc.taskType {
			t.Fatalf("task type = %q, want %q", task.TaskType, tc.taskType)
		}
	}
}

func TestCreateTaskAllowsComposerTurn(t *testing.T) {
	svc, err := NewService(&fakeBeginner{}, db.New(fakeDBTX{}))
	if err != nil {
		t.Fatal(err)
	}

	task, err := svc.CreateTask(context.Background(), CreateTaskParams{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		Role:        "composer",
		ScopeType:   "final_output",
		TaskType:    "composer_turn",
		MaxAttempts: 1,
		Input:       []byte(`{"shot_ids":["shot-01"]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.Role != "composer" || task.TaskType != "composer_turn" {
		t.Fatalf("task = %#v", task)
	}
}

func TestRuntimeScopesAllowAudioPlan(t *testing.T) {
	if !validThreadScope("audio_plan") {
		t.Fatal("audio_plan should be a valid agent thread scope")
	}
	if !validTaskScope("audio_plan") {
		t.Fatal("audio_plan should be a valid agent task scope")
	}
	if !validProducerSignalScope("audio_plan") {
		t.Fatal("audio_plan should be a valid producer signal scope")
	}
}

func TestListQueuedProducerTasksRejectsMissingWorkspace(t *testing.T) {
	svc, err := NewService(&fakeBeginner{}, db.New(fakeDBTX{}))
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ListQueuedProducerTasks(context.Background(), pgtype.UUID{}, 10)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestRuntimeListAgentThreadsByWorkspaceRejectsInvalidWorkspace(t *testing.T) {
	svc, err := NewService(&fakeBeginner{}, db.New(fakeDBTX{}))
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ListAgentThreadsByWorkspace(context.Background(), pgtype.UUID{}, false)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestRuntimeGetThreadForWorkspaceRejectsInvalidThread(t *testing.T) {
	svc, err := NewService(&fakeBeginner{}, db.New(fakeDBTX{}))
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.GetThreadForWorkspace(context.Background(), pgtype.UUID{}, uuidWithByte(1))
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestRuntimeListThreadMessagesRejectsInvalidThread(t *testing.T) {
	svc, err := NewService(&fakeBeginner{}, db.New(fakeDBTX{}))
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ListThreadMessages(context.Background(), pgtype.UUID{}, 0, 100)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestCheckpointKeyUsesWorkspaceThreadAndTask(t *testing.T) {
	key := CheckpointKey(uuidWithByte(1), uuidWithByte(2), uuidWithByte(3))

	if key != "agent:01000000-0000-0000-0000-000000000000:02000000-0000-0000-0000-000000000000:03000000-0000-0000-0000-000000000000" {
		t.Fatalf("checkpoint key = %q", key)
	}
}

func uuidWithByte(b byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{b}, Valid: true}
}

type fakeBeginner struct {
	tx *fakeTx
}

func (f *fakeBeginner) Begin(context.Context) (pgx.Tx, error) {
	f.tx = &fakeTx{}
	return f.tx, nil
}

type fakeDBTX struct{}

func (fakeDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (fakeDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (fakeDBTX) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	if strings.Contains(query, "INSERT INTO agent_task") {
		return fakeRow{values: []any{
			uuidWithByte(8),
			args[0].(pgtype.UUID),
			args[1].(pgtype.UUID),
			args[2].(string),
			args[3].(string),
			args[4].(pgtype.UUID),
			args[5].(string),
			"queued",
			int32(0),
			args[6].(int32),
			args[7].([]byte),
			[]byte("{}"),
			pgtype.Text{},
			pgtype.Text{},
			pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true},
			pgtype.Timestamptz{},
			pgtype.Timestamptz{},
			pgtype.UUID{},
			"",
			"",
		}}
	}
	return fakeRow{}
}

type fakeTx struct {
	committed          bool
	rolledBack         bool
	nextSeqThreadID    pgtype.UUID
	createdMessageType string
}

func (f *fakeTx) Begin(context.Context) (pgx.Tx, error) { return nil, nil }
func (f *fakeTx) Commit(context.Context) error {
	f.committed = true
	return nil
}
func (f *fakeTx) Rollback(context.Context) error {
	f.rolledBack = !f.committed
	return nil
}
func (f *fakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (f *fakeTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (f *fakeTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (f *fakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (f *fakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (f *fakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (f *fakeTx) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	if len(args) == 1 {
		f.nextSeqThreadID = args[0].(pgtype.UUID)
		return fakeRow{values: []any{int64(1)}}
	}

	f.createdMessageType = args[4].(string)
	return fakeRow{values: []any{
		uuidWithByte(9),
		args[0].(pgtype.UUID),
		args[1].(pgtype.UUID),
		args[2].(int64),
		args[3].(string),
		args[4].(string),
		args[5].([]byte),
		args[6].([]byte),
		args[7].(pgtype.UUID),
		args[8].(pgtype.UUID),
		pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true},
	}}
}
func (f *fakeTx) Conn() *pgx.Conn { return nil }

type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for i := range dest {
		switch d := dest[i].(type) {
		case *pgtype.UUID:
			*d = r.values[i].(pgtype.UUID)
		case *int64:
			*d = r.values[i].(int64)
		case *int32:
			*d = r.values[i].(int32)
		case *string:
			*d = r.values[i].(string)
		case *[]byte:
			*d = r.values[i].([]byte)
		case *pgtype.Timestamptz:
			*d = r.values[i].(pgtype.Timestamptz)
		case *pgtype.Text:
			*d = r.values[i].(pgtype.Text)
		default:
			return errors.New("unsupported fake scan destination")
		}
	}
	return nil
}
