package playbacktest

import (
	"testing"
	"time"
)

func TestFakeEngineMeetsContract(t *testing.T) {
	RunEngineContract(t, func(*testing.T) Harness {
		engine := &Engine{Simulate: true}
		return Harness{Engine: engine, EndTrack: func(t *testing.T) {
			deadline := time.Now().Add(contractTimeout)
			for engine.LastSession() == nil {
				if time.Now().After(deadline) {
					t.Fatal("no session started")
				}
				time.Sleep(time.Millisecond)
			}
			engine.LastSession().EndTrack()
		}}
	})
}
