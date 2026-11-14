package rtspd

import (
	"github.com/ygelfand/libcountertop/pkg/network/digest"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

const Group setting.Group = "RTSP"

const credentialsMost = 64

func NewTable() *setting.Table[schema.RTSP] {
	password := setting.Words(
		setting.Setting[schema.RTSP]{
			Name:  "password",
			Group: Group,
			Icon:  "mdi:form-textbox-password",
		},
		credentialsMost,
		func(c *schema.RTSP) *string { return &c.Password },
	)
	password.Kind = setting.Secret
	return setting.NewTable("rtsp", []setting.Group{Group}, []setting.Setting[schema.RTSP]{
		setting.Flag(
			setting.Setting[schema.RTSP]{
				Name:  "enabled",
				Group: Group,
				ID:    "rtsp",
				Icon:  "mdi:cctv",
			},
			func(c *schema.RTSP) *bool { return &c.Enabled },
		),
		setting.Flag(
			setting.Setting[schema.RTSP]{Name: "audio", Group: Group, Icon: "mdi:microphone"},
			func(c *schema.RTSP) *bool { return &c.Audio },
		),
		setting.Words(
			setting.Setting[schema.RTSP]{Name: "username", Group: Group, Icon: "mdi:account"},
			credentialsMost,
			func(c *schema.RTSP) *string { return &c.Username },
		),
		password,
	}, setting.Messages{Text: say.T, Missing: say.Missing})
}

func Controls(
	device uint32,
	read func() schema.RTSP,
	apply func(schema.RTSP) error,
) *setting.Controls[schema.RTSP] {
	return &setting.Controls[schema.RTSP]{
		Table:  NewTable(),
		Device: device,
		Read:   read,
		Save: func(s setting.Setting[schema.RTSP], v string) error {
			at := read()
			if err := s.Write(&at, v); err != nil {
				return err
			}
			return apply(at)
		},
	}
}

func Credentials(read func() schema.RTSP) digest.Credentials {
	return func() (string, string) {
		at := read()
		return at.Username, at.Password
	}
}
