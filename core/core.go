package core

import (
	"os"
	"path"
	"strconv"

	log "github.com/sirupsen/logrus"

	"github.com/gabek/owncast/config"
	"github.com/gabek/owncast/core/chat"
	"github.com/gabek/owncast/core/ffmpeg"
	"github.com/gabek/owncast/core/storageproviders"
	"github.com/gabek/owncast/models"
	"github.com/gabek/owncast/utils"
	"github.com/gabek/owncast/yp"
)

var (
	_stats        *models.Stats
	_storage      models.StorageProvider
	_transcoder   *ffmpeg.Transcoder
	_cleanupTimer *time.Timer
	_yp           *yp.YP
	_broadcaster  *models.Broadcaster
)

var handler ffmpeg.HLSHandler
var fileWriter = ffmpeg.FileWriterReceiverService{}

//Start starts up the core processing
func Start() error {
	resetDirectories()

	if err := setupStats(); err != nil {
		log.Error("failed to setup the stats")
		return err
	}

	if err := setupStorage(); err != nil {
		log.Error("failed to setup the storage")
		return err
	}

	// The HLS handler takes the written HLS playlists and segments
	// and makes storage decisions.  It's rather simple right now
	// but will play more useful when recordings come into play.
	handler = ffmpeg.HLSHandler{}
	handler.Storage = _storage
	fileWriter.SetupFileWriterReceiverService(&handler)

	if err := createInitialOfflineState(); err != nil {
		log.Error("failed to create the initial offline state")
		return err
	}

	if config.Config.YP.Enabled {
		_yp = yp.NewYP(GetStatus)
	} else {
		yp.DisplayInstructions()
	}

	if config.Config.S3.Enabled {
		_storage = &storageproviders.S3Storage{}
	} else {
		_storage = &storageproviders.LocalStorage{}
	}

	chat.Setup(ChatListenerImpl{})

	return nil
}

func createInitialOfflineState() error {
	// Provide default files
	if !utils.DoesFileExists("webroot/thumbnail.jpg") {
		if err := utils.Copy("static/logo.png", "webroot/thumbnail.jpg"); err != nil {
			return err
		}
	}

	SetStreamAsDisconnected()

	return nil
}

func startCleanupTimer() {
	_cleanupTimer = time.NewTimer(5 * time.Minute)
	go func() {
		for {
			select {
			case <-_cleanupTimer.C:
				// Reset the session count since the session is over
				_stats.SessionMaxViewerCount = 0
				resetDirectories()
				ffmpeg.ShowStreamOfflineState()
			}
		}
	}()
}

// StopCleanupTimer will stop the previous cleanup timer
func stopCleanupTimer() {
	if _cleanupTimer != nil {
		_cleanupTimer.Stop()
	}
}

func resetDirectories() {
	log.Trace("Resetting file directories to a clean slate.")

	// Wipe the public, web-accessible hls data directory
	os.RemoveAll(config.Config.GetPublicHLSSavePath())
	os.RemoveAll(config.Config.GetPrivateHLSSavePath())
	os.MkdirAll(config.Config.GetPublicHLSSavePath(), 0777)
	os.MkdirAll(config.Config.GetPrivateHLSSavePath(), 0777)

	// Remove the previous thumbnail
	os.Remove("webroot/thumbnail.jpg")

	// Create private hls data dirs
	if len(config.Config.VideoSettings.StreamQualities) != 0 {
		for index := range config.Config.VideoSettings.StreamQualities {
			os.MkdirAll(path.Join(config.Config.GetPrivateHLSSavePath(), strconv.Itoa(index)), 0777)
			os.MkdirAll(path.Join(config.Config.GetPublicHLSSavePath(), strconv.Itoa(index)), 0777)
		}
	} else {
		os.MkdirAll(path.Join(config.Config.GetPrivateHLSSavePath(), strconv.Itoa(0)), 0777)
		os.MkdirAll(path.Join(config.Config.GetPublicHLSSavePath(), strconv.Itoa(0)), 0777)
	}
}
