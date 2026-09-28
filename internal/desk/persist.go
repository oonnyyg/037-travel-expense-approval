package desk

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const stateFileName = "state.json"

type persistedState struct {
	Threshold int64             `json:"threshold"`
	Initialed bool              `json:"initialed"`
	Counter   int64             `json:"counter"`
	Sheets    []*Sheet          `json:"sheets"`
	Occupied  map[string]string `json:"occupied"`
}

func (d *Desk) statePath() string {
	return filepath.Join(d.dir, stateFileName)
}

// persistLocked 在持锁状态下把全部状态原子落盘。
func (d *Desk) persistLocked() error {
	if d.dir == "" {
		return nil
	}
	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return err
	}
	st := persistedState{
		Threshold: d.threshold,
		Initialed: d.initialed,
		Counter:   d.counter,
		Sheets:    make([]*Sheet, 0, len(d.sheets)),
		Occupied:  d.occupied,
	}
	for _, s := range d.sheets {
		st.Sheets = append(st.Sheets, s)
	}
	data, err := json.MarshalIndent(&st, "", "  ")
	if err != nil {
		return err
	}
	final := d.statePath()
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// Load 从数据目录恢复状态；目录或文件不存在时返回空 Desk。
func Load(dataDir string) (*Desk, error) {
	d := New(dataDir)
	data, err := os.ReadFile(filepath.Join(dataDir, stateFileName))
	if os.IsNotExist(err) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	var st persistedState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	d.threshold = st.Threshold
	d.initialed = st.Initialed
	d.counter = st.Counter
	d.occupied = st.Occupied
	if d.occupied == nil {
		d.occupied = make(map[string]string)
	}
	for _, s := range st.Sheets {
		if s.Items == nil {
			s.Items = []Item{}
		}
		if s.History == nil {
			s.History = []HistoryEntry{}
		}
		d.sheets[s.ID] = s
	}
	return d, nil
}
