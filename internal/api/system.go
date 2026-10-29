package api

type systemInfo struct {
	CPUUsage float64 `json:"cpu_usage"` // percent 0-100
	MemTotal uint64  `json:"mem_total"` // bytes
	MemUsed  uint64  `json:"mem_used"`  // bytes
}

func getSystemInfo() systemInfo {
	memTotal, memUsed := getMemoryInfo()
	return systemInfo{
		CPUUsage: getCPUUsage(),
		MemTotal: memTotal,
		MemUsed:  memUsed,
	}
}
