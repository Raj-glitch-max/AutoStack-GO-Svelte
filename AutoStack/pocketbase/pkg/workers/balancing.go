package workers

// LoadReport summarizes current load distribution across workers.
type LoadReport struct {
	TotalWorkers     int
	AvailableWorkers int
	DegradedWorkers  int
	DrainingWorkers  int
	TotalActiveTasks int
	PerWorker        []WorkerLoad
}

// WorkerLoad is the per-worker load summary.
type WorkerLoad struct {
	WorkerID    string
	State       WorkerState
	ActiveTasks int
}

// ComputeLoad produces a load report from a worker list and heartbeat log.
func ComputeLoad(workers []Worker, hb *HeartbeatLog) LoadReport {
	report := LoadReport{TotalWorkers: len(workers)}
	report.PerWorker = make([]WorkerLoad, 0, len(workers))

	for _, w := range workers {
		var activeTasks int
		if last, ok := hb.LastHeartbeat(w.WorkerID); ok {
			activeTasks = last.ActiveTasks
		}
		report.TotalActiveTasks += activeTasks
		report.PerWorker = append(report.PerWorker, WorkerLoad{
			WorkerID:    w.WorkerID,
			State:       w.State,
			ActiveTasks: activeTasks,
		})
		switch w.State {
		case WorkerStateOnline:
			report.AvailableWorkers++
		case WorkerStateDegraded:
			report.DegradedWorkers++
		case WorkerStateDraining:
			report.DrainingWorkers++
		}
	}
	return report
}
