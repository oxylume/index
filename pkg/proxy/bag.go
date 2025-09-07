package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oxylume/index/pkg/singleflight"
	"github.com/xssnick/tonutils-storage/storage"
)

type entry struct {
	bag    *Bag
	readyC chan struct{}
	err    error
}

// todo: maybe better use Acquire/Release with internal "lastUsed" and "inUse" states to decide when to evict a bag?
// now it's pretty fragile that there's (small) chance that we can evict a bag that is in use, also I don't like that we track "lastUsed" inside Bag struct
type BagProvider struct {
	connector *storage.Connector
	storage   storage.Storage
	ttl       time.Duration
	bags      map[string]*entry
	mx        sync.Mutex
	closer    context.CancelFunc
}

func NewBagProvider(connector *storage.Connector, storage storage.Storage, ttl time.Duration) *BagProvider {
	p := &BagProvider{
		connector: connector,
		storage:   storage,
		ttl:       ttl,
		bags:      make(map[string]*entry),
	}
	return p
}

func (p *BagProvider) Start(ctx context.Context) {
	ctx, p.closer = context.WithCancel(ctx)
	go p.evictor(ctx)
}

func (p *BagProvider) Close() {
	if p.closer != nil {
		p.closer()
	}
}

func (p *BagProvider) GetBag(ctx context.Context, id []byte) (*Bag, error) {
	bagId := string(id)
	p.mx.Lock()
	if entry, ok := p.bags[bagId]; ok {
		entry.bag.lastUsed.Store(time.Now().Unix())
		p.mx.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-entry.readyC:
			if entry.err != nil {
				return nil, entry.err
			}
			return entry.bag, nil
		}
	}

	torrent := storage.NewTorrent("", p.storage, p.connector)
	torrent.BagID = id
	if err := p.storage.SetTorrent(torrent); err != nil {
		p.mx.Unlock()
		return nil, fmt.Errorf("unable to save torrent: %w", err)
	}
	if err := torrent.Start(true, false, false); err != nil {
		p.mx.Unlock()
		return nil, fmt.Errorf("unable to start torrent: %w", err)
	}
	bag := &Bag{
		torrent: torrent,
	}
	bag.lastUsed.Store(time.Now().Unix())
	entry := &entry{
		bag:    bag,
		readyC: make(chan struct{}),
	}
	p.bags[bagId] = entry
	p.mx.Unlock()

	var err error
	defer close(entry.readyC)
	bag.downloader, err = p.connector.CreateDownloader(ctx, torrent)
	if err != nil {
		p.mx.Lock()
		defer p.mx.Unlock()
		torrent.Stop()
		delete(p.bags, bagId)
		entry.err = err
		return nil, err
	}
	return bag, nil
}

func (p *BagProvider) evictor(ctx context.Context) {
	interval := 1 * time.Minute
	if p.ttl < interval*2 {
		interval = p.ttl / 2
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			p.mx.Lock()
			for id, entry := range p.bags {
				lastUsed := time.Unix(entry.bag.lastUsed.Load(), 0)
				if lastUsed.Add(p.ttl).Before(now) {
					if entry.bag.downloader != nil {
						entry.bag.downloader.Close()
					}
					entry.bag.torrent.Stop()
					delete(p.bags, id)
				}
			}
			p.mx.Unlock()
		}
	}
}

type Bag struct {
	torrent    *storage.Torrent
	downloader storage.TorrentDownloader
	pieceSf    singleflight.Group[uint32, []byte]
	lastUsed   atomic.Int64
}

func (b *Bag) GetFileOffsets(name string) (*storage.FileInfo, error) {
	return b.torrent.GetFileOffsets(name)
}

func (b *Bag) WriteFileTo(ctx context.Context, w io.Writer, file *storage.FileInfo, from uint64, to uint64, workers int) error {
	pieceSize := uint64(b.torrent.Info.PieceSize)
	fromOffset := from + uint64(file.FromPieceOffset)
	fromPiece := file.FromPiece + uint32(fromOffset/pieceSize)
	toOffset := to + uint64(file.FromPieceOffset)
	toPiece := file.FromPiece + uint32(toOffset/pieceSize)

	pieces := make([]byte, b.torrent.Info.PiecesNum())
	for piece := fromPiece; piece <= toPiece; piece++ {
		pieces[piece] = 1
	}
	fetcher := storage.NewPreFetcher(ctx, b.torrent, nil, 32, pieces)
	defer fetcher.Stop()
	for piece := fromPiece; piece <= toPiece; piece++ {
		var data []byte
		var err error
		for {
			b.lastUsed.Store(time.Now().Unix())
			data, err = b.pieceSf.Do(piece, func() ([]byte, error) {
				data, _, err := fetcher.WaitGet(ctx, piece)
				return data, err
			})
			if errors.Is(err, context.Canceled) && ctx.Err() == nil {
				// if it was cancelled and our ctx is still good - retry
				continue
			}
			fetcher.Free(piece)
			break
		}
		if err != nil {
			return fmt.Errorf("unable to download piece: %w", err)
		}
		if piece == toPiece {
			data = data[:toOffset%pieceSize+1]
		}
		if piece == fromPiece {
			data = data[fromOffset%pieceSize:]
		}
		_, err = w.Write(data)
		if err != nil {
			return fmt.Errorf("unable to write piece: %w", err)
		}
		b.lastUsed.Store(time.Now().Unix())
	}
	return nil
}
