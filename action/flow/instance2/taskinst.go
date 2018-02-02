package instance2

import (
	"fmt"
	"errors"
	"runtime/debug"

	"github.com/TIBCOSoftware/flogo-contrib/action/flow/definition"
	"github.com/TIBCOSoftware/flogo-contrib/action/flow/model"
	"github.com/TIBCOSoftware/flogo-lib/core/activity"
	"github.com/TIBCOSoftware/flogo-lib/core/data"
	"github.com/TIBCOSoftware/flogo-lib/logger"
)

func NewTaskInst(inst *Instance, task *definition.Task) *TaskInst {
	var taskInst TaskInst

	taskInst.flowInst = inst
	taskInst.task = task
	return &taskInst
}

type TaskInst struct {
	flowInst *Instance
	task     *definition.Task
	status   model.TaskStatus

	workingData map[string]*data.Attribute

	inScope  data.Scope
	outScope data.Scope

	taskID string //needed for serialization
}

// InputScope get the InputScope of the task instance
func (ti *TaskInst) InputScope() data.Scope {

	if ti.inScope != nil {
		return ti.inScope
	}

	if len(ti.task.ActivityConfig().Ref()) > 0 {

		act := activity.Get(ti.task.ActivityConfig().Ref())
		ti.inScope = NewFixedTaskScope(act.Metadata().Input, ti.task, true)

	} else if ti.task.IsScope() {

		//add flow scope
	}

	return ti.inScope
}

// OutputScope get the InputScope of the task instance
func (ti *TaskInst) OutputScope() data.Scope {

	if ti.outScope != nil {
		return ti.outScope
	}

	if len(ti.task.ActivityConfig().Ref()) > 0 {

		act := activity.Get(ti.task.ActivityConfig().Ref())
		ti.outScope = NewFixedTaskScope(act.Metadata().Output, ti.task, false)

		logger.Debugf("OutputScope: %v\n", ti.outScope)
	} else if ti.task.IsScope() {

		//add flow scope
	}

	return ti.outScope
}

/////////////////////////////////////////
// TaskInst - activity.Context Implementation

func (ti *TaskInst) Host() activity.Host {
	return ti.flowInst
}

// Name implements activity.Context.Name method
func (ti *TaskInst) Name() string {
	return ti.task.Name()
}

// GetInput implements activity.Context.GetInput
func (ti *TaskInst) GetInput(name string) interface{} {

	val, found := ti.InputScope().GetAttr(name)
	if found {
		return val.Value()
	}

	return nil
}

// GetOutput implements activity.Context.GetOutput
func (ti *TaskInst) GetOutput(name string) interface{} {

	val, found := ti.OutputScope().GetAttr(name)
	if found {
		return val.Value()
	}

	return nil
}

// SetOutput implements activity.Context.SetOutput
func (ti *TaskInst) SetOutput(name string, value interface{}) {

	logger.Debugf("SET OUTPUT: %s = %v\n", name, value)
	ti.OutputScope().SetAttrValue(name, value)
}

// TaskName implements activity.Context.TaskName method
// Deprecated
func (ti *TaskInst) TaskName() string {
	return ti.task.Name()
}

/////////////////////////////////////////
// TaskInst - TaskContext Implementation

// Status implements flow.TaskContext.GetState
func (ti *TaskInst) Status() model.TaskStatus {
	return ti.status
}

// SetStatus implements flow.TaskContext.SetStatus
func (ti *TaskInst) SetStatus(status model.TaskStatus) {
	ti.status = status
	ti.flowInst.master.ChangeTracker.trackTaskData(ti.flowInst.subFlowId, &TaskInstChange{ChgType: CtUpd, ID: ti.task.ID(), TaskInst: ti})
}

func (ti *TaskInst) HasWorkingData() bool {
	return ti.workingData != nil
}

func (ti *TaskInst) GetSetting(setting string) (value interface{}, exists bool) {

	value, exists = ti.task.GetSetting(setting)

	if !exists {
		return nil, false
	}

	strValue, ok := value.(string)

	if ok && strValue[0] == '$' {

		v, err := definition.GetDataResolver().Resolve(strValue, ti.flowInst)
		if err != nil {
			return nil, false
		}

		return v, true

	} else {
		return value, true
	}
}

func (ti *TaskInst) AddWorkingData(attr *data.Attribute) {

	if ti.workingData == nil {
		ti.workingData = make(map[string]*data.Attribute)
	}
	ti.workingData[attr.Name()] = attr
}

func (ti *TaskInst) UpdateWorkingData(key string, value interface{}) error {

	if ti.workingData == nil {
		return errors.New("working data '" + key + "' not defined")
	}

	attr, ok := ti.workingData[key]

	if ok {
		attr.SetValue(value)
	} else {
		return errors.New("working data '" + key + "' not defined")
	}

	return nil
}

func (ti *TaskInst) GetWorkingData(key string) (*data.Attribute, bool) {
	if ti.workingData == nil {
		return nil, false
	}

	v, ok := ti.workingData[key]
	return v, ok
}

// Task implements model.TaskContext.Task, by returning the Task associated with this
// TaskInst object
func (ti *TaskInst) Task() *definition.Task {
	return ti.task
}

// GetFromLinkInstances implements model.TaskContext.GetFromLinkInstances
func (ti *TaskInst) GetFromLinkInstances() []model.LinkInstance {

	logger.Debugf("GetFromLinkInstances: task=%v\n", ti.Task)

	links := ti.task.FromLinks()

	numLinks := len(links)

	if numLinks > 0 {
		linkCtxs := make([]model.LinkInstance, numLinks)

		for i, link := range links {
			linkCtxs[i], _ = ti.flowInst.FindOrCreateLinkData(link)
		}
		return linkCtxs
	}

	return nil
}

// GetToLinkInstances implements model.TaskContext.GetToLinkInstances,
func (ti *TaskInst) GetToLinkInstances() []model.LinkInstance {

	logger.Debugf("GetToLinkInstances: task=%v\n", ti.Task)

	links := ti.task.ToLinks()

	numLinks := len(links)

	if numLinks > 0 {
		linkCtxs := make([]model.LinkInstance, numLinks)

		for i, link := range links {
			linkCtxs[i], _ = ti.flowInst.FindOrCreateLinkData(link)
		}
		return linkCtxs
	}

	return nil
}

// EvalLink implements activity.ActivityContext.EvalLink method
func (ti *TaskInst) EvalLink(link *definition.Link) (result bool, err error) {

	logger.Debugf("TaskContext.EvalLink: %d\n", link.ID())

	defer func() {
		if r := recover(); r != nil {
			logger.Warnf("Unhandled Error evaluating link '%s' : %v\n", link.ID(), r)

			// todo: useful for debugging
			logger.Debugf("StackTrace: %s", debug.Stack())

			if err != nil {
				err = fmt.Errorf("%v", r)
			}
		}
	}()

	mgr := ti.flowInst.flowDef.GetLinkExprManager()

	if mgr != nil {
		result, err = mgr.EvalLinkExpr(link, ti.flowInst)
		return result, err
	}

	return true, nil
}

// HasActivity implements activity.ActivityContext.HasActivity method
func (ti *TaskInst) HasActivity() bool {
	return activity.Get(ti.task.ActivityConfig().Ref()) != nil
}

// EvalActivity implements activity.ActivityContext.EvalActivity method
func (ti *TaskInst) EvalActivity() (done bool, evalErr error) {

	defer func() {
		if r := recover(); r != nil {
			logger.Warnf("Unhandled Error executing activity '%s'[%s] : %v\n", ti.task.Name(), ti.task.ActivityConfig().Ref(), r)

			// todo: useful for debugging
			logger.Debugf("StackTrace: %s", debug.Stack())

			if evalErr == nil {
				evalErr = NewActivityEvalError(ti.task.Name(), "unhandled", fmt.Sprintf("%v", r))
				done = false
			}
		}
		if evalErr != nil {
			logger.Errorf("Execution failed for Activity[%s] in Flow[%s] - %s", ti.task.Name(), ti.flowInst.flowDef.Name(), evalErr.Error())
		}
	}()

	eval := true

	if ti.task.ActivityConfig().InputMapper() != nil {

		err := applyInputMapper(ti)

		if err != nil {

			evalErr = NewActivityEvalError(ti.task.Name(), "mapper", err.Error())
			return false, evalErr
		}

		eval = applyInputInterceptor(ti)
	}

	if eval {

		act := activity.Get(ti.task.ActivityConfig().Ref())
		done, evalErr = act.Eval(ti)

		if evalErr != nil {
			e, ok := evalErr.(*activity.Error)
			if ok {
				e.SetActivityName(ti.task.Name())
			}

			return false, evalErr
		}
	} else {
		done = true
	}

	if done {

		if ti.task.ActivityConfig().OutputMapper() != nil {
			applyOutputInterceptor(ti)

			appliedMapper, err := applyOutputMapper(ti)

			if err != nil {
				evalErr = NewActivityEvalError(ti.task.Name(), "mapper", err.Error())
				return done, evalErr
			}

			if !appliedMapper && !ti.task.IsScope() {

				logger.Debug("Mapper not applied")
			}
		}
	}

	return done, nil
}

//// Failed marks the Activity as failed
//func (td *TaskInst) Failed(err error) {
//
//	errorMsgAttr := "[A" + td.task.ID() + "._errorMsg]"
//	td.inst.AddAttr(errorMsgAttr, data.STRING, err.Error())
//	errorMsgAttr2 := "[activity." + td.task.ID() + "._errorMsg]"
//	td.inst.AddAttr(errorMsgAttr2, data.STRING, err.Error())
//}
