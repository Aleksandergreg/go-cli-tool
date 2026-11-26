package mission

import "testing"

func TestKubernetesWorldAndSchema(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	worlds := catalog.Worlds(TrackKubernetes)
	if len(worlds) != 1 || worlds[0].Name != "Checkout on Call" || len(worlds[0].Missions) != 3 {
		t.Fatal("missing Kubernetes world")
	}
	item := worlds[0].Missions[2]
	for _, change := range []func(*Mission){
		func(m *Mission) { m.Track = TrackLinux },
		func(m *Mission) { m.StartDir = "/" },
		func(m *Mission) { m.Kubernetes[0].Name = m.Kubernetes[1].Name },
		func(m *Mission) { m.Kubernetes[0].Kind = "Secret" },
		func(m *Mission) { m.Validation.All[0].Value = "foreign" },
		func(m *Mission) { *m.Validation.All[0].Count = 4 },
	} {
		bad := cloneMission(item)
		change(&bad)
		if validateMission(bad) == nil {
			t.Fatal("invalid Kubernetes mission accepted")
		}
	}
	if err := validateMission(item); err != nil {
		t.Fatal(err)
	}
}
