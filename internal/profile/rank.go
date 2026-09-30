package profile

var rankThresholds = []struct {
	name string
	xp   int
}{
	{name: "Intern", xp: 0},
	{name: "Operator", xp: 100},
	{name: "Junior Sysadmin", xp: 250},
	{name: "Sysadmin", xp: 450},
	{name: "SRE", xp: 650},
	{name: "Senior SRE", xp: 1100},
}

func (p Profile) Level() int { return p.XP/100 + 1 }

func (p Profile) Rank() string {
	for index := len(rankThresholds) - 1; index >= 0; index-- {
		if p.XP >= rankThresholds[index].xp {
			return rankThresholds[index].name
		}
	}
	return rankThresholds[0].name
}

func (p Profile) NextRank() (string, int, bool) {
	for _, threshold := range rankThresholds[1:] {
		if p.XP < threshold.xp {
			return threshold.name, threshold.xp - p.XP, true
		}
	}
	return "", 0, false
}
