package httpapi

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ali96adil/StageCore/internal/camerarelay"
	"github.com/ali96adil/StageCore/internal/projectaccess"
	"github.com/ali96adil/StageCore/internal/userauth"
)

const cameraViewerPermissionsPath = projectRoute + "/camera-relay/stream"

type ProjectCameraViewer struct {
	projectID string
	upstream  string
	tokenFile string
	client    *http.Client
}

func NewProjectCameraViewer(projectID, address, tokenFile string) (*ProjectCameraViewer, error) {
	host, port, err := net.SplitHostPort(address)
	number, portErr := strconv.Atoi(port)
	if strings.TrimSpace(projectID) == "" || err != nil || !net.ParseIP(host).IsLoopback() ||
		portErr != nil || number < 1 || number > 65535 {
		return nil, errors.New("camera viewing requires a project and a numeric loopback host:port")
	}
	if _, err := camerarelay.ReadViewerToken(tokenFile); err != nil {
		return nil, err
	}
	upstream := url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(number)), Path: "/api/v0/stream"}
	return &ProjectCameraViewer{
		projectID: strings.TrimSpace(projectID), upstream: upstream.String(), tokenFile: tokenFile,
		client: &http.Client{
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
				ResponseHeaderTimeout: 5 * time.Second,
				DisableCompression:    true,
				DisableKeepAlives:     true,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// WithOperatorCameraViewer scopes this opt-in route even when the general
// project qualification router is disabled.
func WithOperatorCameraViewer(auth *userauth.Service, access *projectaccess.Service, viewer *ProjectCameraViewer) Option {
	return func(s *Server) {
		if auth == nil || access == nil || viewer == nil {
			return
		}
		s.mux.HandleFunc("GET /api/v1/projects/{project_id}/camera-relay/stream", withAuthenticatedSession(auth, false,
			func(w http.ResponseWriter, r *http.Request, session userauth.Session, token string) {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				if r.PathValue("project_id") != viewer.projectID {
					writeProjectQualificationError(w, projectaccess.ErrNotFound)
					return
				}
				check := func(ctx context.Context) error {
					ctx, cancel := context.WithTimeout(ctx, time.Second)
					defer cancel()
					current, err := auth.ValidatePassive(ctx, token)
					if err != nil || current.ID != session.ID || current.User.ID != session.User.ID {
						return projectaccess.ErrDenied
					}
					return access.CheckAll(ctx, session.User.ID, viewer.projectID, projectaccess.ProjectView, projectaccess.MediaRead)
				}
				if !writeProjectQualificationError(w, check(r.Context())) {
					return
				}
				if r.Method == http.MethodHead {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				viewer.stream(w, r, check)
			}))
	}
}

func (v *ProjectCameraViewer) stream(w http.ResponseWriter, r *http.Request, check func(context.Context) error) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if check(ctx) != nil {
					cancel()
					return
				}
			}
		}
	}()
	unavailable := func() {
		if ctx.Err() == nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error_code": "CAMERA_RELAY_UNAVAILABLE"})
		}
	}
	token, err := camerarelay.ReadViewerToken(v.tokenFile)
	if err != nil {
		unavailable()
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.upstream, nil)
	if err != nil {
		unavailable()
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := v.client.Do(req)
	if err != nil {
		unavailable()
		return
	}
	defer resp.Body.Close()
	kind, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	boundary := params["boundary"]
	if resp.StatusCode != http.StatusOK || err != nil || kind != "multipart/x-mixed-replace" || boundary == "" || len(boundary) > 70 {
		unavailable()
		return
	}

	frames := multipart.NewReader(resp.Body, boundary)
	output := multipart.NewWriter(w)
	firstFrame := true
	control := http.NewResponseController(w)
	defer control.SetWriteDeadline(time.Time{})
	const maxFrame = 4 << 20
	for ctx.Err() == nil {
		frame, err := frames.NextRawPart()
		if err != nil || frame.Header.Get("Content-Type") != "image/jpeg" {
			return
		}
		length, err := strconv.Atoi(frame.Header.Get("Content-Length"))
		if err != nil || length < 4 || length > maxFrame {
			return
		}
		jpeg := make([]byte, length)
		_, err = io.ReadFull(frame, jpeg)
		if err != nil || jpeg[0] != 0xff || jpeg[1] != 0xd8 ||
			jpeg[len(jpeg)-2] != 0xff || jpeg[len(jpeg)-1] != 0xd9 || ctx.Err() != nil {
			return
		}
		accessErr := check(ctx)
		if accessErr != nil || ctx.Err() != nil {
			if firstFrame && accessErr != nil {
				writeProjectQualificationError(w, accessErr)
			}
			cancel()
			return
		}
		if firstFrame {
			w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+output.Boundary())
			firstFrame = false
		}
		if err := control.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
			return
		}
		part, err := output.CreatePart(textproto.MIMEHeader{"Content-Type": {"image/jpeg"}, "Content-Length": {strconv.Itoa(length)}})
		if err != nil {
			return
		}
		if _, err := part.Write(jpeg); err != nil || control.Flush() != nil || control.SetWriteDeadline(time.Time{}) != nil {
			return
		}
	}
}
