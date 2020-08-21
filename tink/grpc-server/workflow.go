package grpcserver

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"text/template"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/tinkerbell/tink/db"
	"github.com/tinkerbell/tink/metrics"
	"github.com/tinkerbell/tink/protos/workflow"
	workflowpb "github.com/tinkerbell/tink/protos/workflow"
)

var state = map[int32]workflow.State{
	0: workflow.State_PENDING,
	1: workflow.State_RUNNING,
	2: workflow.State_FAILED,
	3: workflow.State_TIMEOUT,
	4: workflow.State_SUCCESS,
}

// CreateWorkflow implements workflow.CreateWorkflow
func (s *server) CreateWorkflow(ctx context.Context, in *workflow.CreateRequest) (*workflow.CreateResponse, error) {
	logger.Info("createworkflow")
	labels := prometheus.Labels{"method": "CreateWorkflow", "op": ""}
	metrics.CacheInFlight.With(labels).Inc()
	defer metrics.CacheInFlight.With(labels).Dec()
	msg := ""
	labels["op"] = "createworkflow"
	msg = "creating a new workflow"
	id := uuid.NewV4()
	fn := func() error {
		wf := db.Workflow{
			ID:       id.String(),
			Template: in.Template,
			Hardware: in.Hardware,
			State:    workflow.State_value[workflow.State_PENDING.String()],
		}
		data, err := createYaml(ctx, s.db, in.Template, in.Hardware)
		if err != nil {
			return errors.Wrap(err, "Failed to create Yaml")
		}
		err = s.db.CreateWorkflow(ctx, wf, data, id)
		if err != nil {
			return err
		}
		return nil
	}

	metrics.CacheTotals.With(labels).Inc()
	timer := prometheus.NewTimer(metrics.CacheDuration.With(labels))
	defer timer.ObserveDuration()

	logger.Info(msg)
	err := fn()
	if err != nil {
		metrics.CacheErrors.With(labels).Inc()
		l := logger
		if pqErr := db.Error(err); pqErr != nil {
			l = l.With("detail", pqErr.Detail, "where", pqErr.Where)
		}
		l.Error(err)
		return &workflow.CreateResponse{}, err
	}
	l := logger.With("workflowID", id.String())
	l.Info("done " + msg)
	return &workflow.CreateResponse{Id: id.String()}, err
}

// GetWorkflow implements workflow.GetWorkflow
func (s *server) GetWorkflow(ctx context.Context, in *workflow.GetRequest) (*workflow.Workflow, error) {
	logger.Info("getworkflow")
	labels := prometheus.Labels{"method": "GetWorkflow", "op": ""}
	metrics.CacheInFlight.With(labels).Inc()
	defer metrics.CacheInFlight.With(labels).Dec()

	msg := ""
	labels["op"] = "get"
	msg = "getting a workflow"

	fn := func() (db.Workflow, error) { return s.db.GetWorkflow(ctx, in.Id) }
	metrics.CacheTotals.With(labels).Inc()
	timer := prometheus.NewTimer(metrics.CacheDuration.With(labels))
	defer timer.ObserveDuration()

	logger.Info(msg)
	w, err := fn()
	if err != nil {
		metrics.CacheErrors.With(labels).Inc()
		l := logger
		if pqErr := db.Error(err); pqErr != nil {
			l = l.With("detail", pqErr.Detail, "where", pqErr.Where)
		}
		l.Error(err)
	}
	yamlData, err := createYaml(ctx, s.db, w.Template, w.Hardware)
	if err != nil {
		return &workflow.Workflow{}, err
	}
	wf := &workflow.Workflow{
		Id:       w.ID,
		Template: w.Template,
		Hardware: w.Hardware,
		State:    state[w.State],
		Data:     yamlData,
	}
	l := logger.With("workflowID", w.ID)
	l.Info("done " + msg)
	return wf, err
}

// DeleteWorkflow implements workflow.DeleteWorkflow
func (s *server) DeleteWorkflow(ctx context.Context, in *workflow.GetRequest) (*workflow.Empty, error) {
	logger.Info("deleteworkflow")
	labels := prometheus.Labels{"method": "DeleteWorkflow", "op": ""}
	metrics.CacheInFlight.With(labels).Inc()
	defer metrics.CacheInFlight.With(labels).Dec()

	msg := ""
	labels["op"] = "delete"
	l := logger.With("workflowID", in.GetId())
	msg = "deleting a workflow"
	fn := func() error {
		// update only if not in running state
		return s.db.DeleteWorkflow(ctx, in.Id, workflow.State_value[workflow.State_RUNNING.String()])
	}

	metrics.CacheTotals.With(labels).Inc()
	timer := prometheus.NewTimer(metrics.CacheDuration.With(labels))
	defer timer.ObserveDuration()

	l.Info(msg)
	err := fn()
	if err != nil {
		metrics.CacheErrors.With(labels).Inc()
		l := logger
		if pqErr := db.Error(err); pqErr != nil {
			l = l.With("detail", pqErr.Detail, "where", pqErr.Where)
		}
		l.Error(err)
	}
	l.Info("done " + msg)
	return &workflow.Empty{}, err
}

// ListWorkflows implements workflow.ListWorkflows
func (s *server) ListWorkflows(_ *workflow.Empty, stream workflow.WorkflowSvc_ListWorkflowsServer) error {
	logger.Info("listworkflows")
	labels := prometheus.Labels{"method": "ListWorkflows", "op": "list"}
	metrics.CacheTotals.With(labels).Inc()
	metrics.CacheInFlight.With(labels).Inc()
	defer metrics.CacheInFlight.With(labels).Dec()

	s.dbLock.RLock()
	ready := s.dbReady
	s.dbLock.RUnlock()
	if !ready {
		metrics.CacheStalls.With(labels).Inc()
		return errors.New("DB is not ready")
	}

	timer := prometheus.NewTimer(metrics.CacheDuration.With(labels))
	defer timer.ObserveDuration()
	err := s.db.ListWorkflows(func(w db.Workflow) error {
		wf := &workflowpb.Workflow{
			Id:        w.ID,
			Template:  w.Template,
			Hardware:  w.Hardware,
			CreatedAt: w.CreatedAt,
			UpdatedAt: w.UpdatedAt,
		}
		return stream.Send(wf)
	})

	if err != nil {
		metrics.CacheErrors.With(labels).Inc()
		return err
	}

	metrics.CacheHits.With(labels).Inc()
	return nil
}

func (s *server) GetWorkflowContext(ctx context.Context, in *workflow.GetRequest) (*workflow.WorkflowContext, error) {
	logger.Info("GetworkflowContext")
	labels := prometheus.Labels{"method": "GetWorkflowContext", "op": ""}
	metrics.CacheInFlight.With(labels).Inc()
	defer metrics.CacheInFlight.With(labels).Dec()

	msg := ""
	labels["op"] = "get"
	msg = "getting a workflow"

	fn := func() (*workflowpb.WorkflowContext, error) { return s.db.GetWorkflowContexts(ctx, in.Id) }
	metrics.CacheTotals.With(labels).Inc()
	timer := prometheus.NewTimer(metrics.CacheDuration.With(labels))
	defer timer.ObserveDuration()

	logger.Info(msg)
	w, err := fn()
	if err != nil {
		metrics.CacheErrors.With(labels).Inc()
		l := logger
		if pqErr := db.Error(err); pqErr != nil {
			l = l.With("detail", pqErr.Detail, "where", pqErr.Where)
		}
		l.Error(err)
	}
	wf := &workflow.WorkflowContext{
		WorkflowId:           w.WorkflowId,
		CurrentWorker:        w.CurrentWorker,
		CurrentTask:          w.CurrentTask,
		CurrentAction:        w.CurrentAction,
		CurrentActionIndex:   w.CurrentActionIndex,
		CurrentActionState:   workflow.ActionState(w.CurrentActionState),
		TotalNumberOfActions: w.TotalNumberOfActions,
	}
	l := logger.With(
		"workflowID", wf.GetWorkflowId(),
		"currentWorker", wf.GetCurrentWorker(),
		"currentTask", wf.GetCurrentTask(),
		"currentAction", wf.GetCurrentAction(),
		"currentActionIndex", strconv.FormatInt(wf.GetCurrentActionIndex(), 10),
		"currentActionState", wf.GetCurrentActionState(),
		"totalNumberOfActions", wf.GetTotalNumberOfActions(),
	)
	l.Info("done " + msg)
	return wf, err
}

// ShowWorflowevents  implements workflow.ShowWorflowEvents
func (s *server) ShowWorkflowEvents(req *workflow.GetRequest, stream workflow.WorkflowSvc_ShowWorkflowEventsServer) error {
	logger.Info("List workflows Events")
	labels := prometheus.Labels{"method": "ShowWorkflowEvents", "op": "list"}
	metrics.CacheTotals.With(labels).Inc()
	metrics.CacheInFlight.With(labels).Inc()
	defer metrics.CacheInFlight.With(labels).Dec()

	s.dbLock.RLock()
	ready := s.dbReady
	s.dbLock.RUnlock()
	if !ready {
		metrics.CacheStalls.With(labels).Inc()
		return errors.New("DB is not ready")
	}

	timer := prometheus.NewTimer(metrics.CacheDuration.With(labels))
	defer timer.ObserveDuration()
	err := s.db.ShowWorkflowEvents(req.Id, func(w *workflowpb.WorkflowActionStatus) error {
		wfs := &workflow.WorkflowActionStatus{
			WorkerId:     w.WorkerId,
			TaskName:     w.TaskName,
			ActionName:   w.ActionName,
			ActionStatus: workflow.ActionState(w.ActionStatus),
			Seconds:      w.Seconds,
			Message:      w.Message,
			CreatedAt:    w.CreatedAt,
		}
		return stream.Send(wfs)
	})

	if err != nil {
		metrics.CacheErrors.With(labels).Inc()
		return err
	}
	logger.Info("done listing workflows events")
	metrics.CacheHits.With(labels).Inc()
	return nil
}

func createYaml(ctx context.Context, db db.Database, temp string, devices string) (string, error) {
	_, tempData, err := db.GetTemplate(ctx, temp)
	if err != nil {
		return "", err
	}
	return renderTemplate(string(tempData), []byte(devices))
}

func renderTemplate(tempData string, devices []byte) (string, error) {
	var hardware map[string]interface{}
	err := json.Unmarshal(devices, &hardware)
	if err != nil {
		logger.Error(err)
		return "", nil
	}

	t := template.New("workflow-template")
	_, err = t.Parse(string(tempData))
	if err != nil {
		logger.Error(err)
		return "", nil
	}

	buf := new(bytes.Buffer)
	err = t.Execute(buf, hardware)
	if err != nil {
		return "", nil
	}
	return buf.String(), nil
}
