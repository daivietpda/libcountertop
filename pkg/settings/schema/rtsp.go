package schema

type RTSP struct {
	Enabled  bool   `json:"enabled"`
	Audio    bool   `json:"audio"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func DefaultRTSP() RTSP { return RTSP{Audio: true, Username: "admin"} }
