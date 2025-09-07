package proxy

import (
	"fmt"
	"sync"

	"github.com/xssnick/tonutils-go/adnl/keys"
	"github.com/xssnick/tonutils-go/tl"
	"github.com/xssnick/tonutils-storage/storage"
)

type MemoryStorage struct {
	torrents map[string]*storage.Torrent
	mx       sync.RWMutex
}

func (s *MemoryStorage) GetFS() storage.FS {
	panic("not implemented")
}

func (s *MemoryStorage) GetAll() []*storage.Torrent {
	s.mx.RLock()
	defer s.mx.RUnlock()

	res := make([]*storage.Torrent, 0, len(s.torrents))
	for _, torrent := range s.torrents {
		res = append(res, torrent)
	}
	return res
}

func (s *MemoryStorage) GetTorrentByOverlay(overlay []byte) *storage.Torrent {
	s.mx.RLock()
	defer s.mx.RUnlock()

	if len(overlay) != 32 {
		return nil
	}
	return s.torrents[string(overlay)]
}

func (s *MemoryStorage) SetTorrent(torrent *storage.Torrent) error {
	id, err := tl.Hash(keys.PublicKeyOverlay{
		Key: torrent.BagID,
	})
	if err != nil {
		return err
	}

	s.mx.Lock()
	defer s.mx.Unlock()

	s.torrents[string(id)] = torrent
	return nil
}

func (s *MemoryStorage) SetActiveFiles(bagId []byte, ids []uint32) error {
	panic("not implemented")
}

func (s *MemoryStorage) GetActiveFiles(bagId []byte) ([]uint32, error) {
	panic("not implemented")
}

func (s *MemoryStorage) GetPiece(bagId []byte, id uint32) (*storage.PieceInfo, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *MemoryStorage) RemovePiece(bagId []byte, id uint32) error {
	return nil
}

func (s *MemoryStorage) SetPiece(bagId []byte, id uint32, p *storage.PieceInfo) error {
	return nil
}

func (s *MemoryStorage) PiecesMask(bagId []byte, num uint32) []byte {
	return make([]byte, (num+7)/8)
}

func (s *MemoryStorage) UpdateUploadStats(bagId []byte, val uint64) error {
	return nil
}

func (s *MemoryStorage) VerifyOnStartup() bool {
	return false
}

func (s *MemoryStorage) GetForcedPieceSize() uint32 {
	return 0
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		torrents: map[string]*storage.Torrent{},
	}
}
