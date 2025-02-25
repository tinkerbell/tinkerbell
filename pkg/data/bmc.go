package data

type BMCMachine struct {
	Host          string
	User          string
	Pass          string
	Port          int
	CipherSuite   string
	SSHPublicKeys []string
}
