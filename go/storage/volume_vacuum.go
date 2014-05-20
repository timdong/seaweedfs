package storage

import (
	"code.google.com/p/weed-fs/go/glog"
	"fmt"
	"os"
)

func (v *Volume) garbageLevel() float64 {
	return float64(v.nm.DeletedSize()) / float64(v.ContentSize())
}

func (v *Volume) Compact() error {
	v.accessLock.Lock()
	defer v.accessLock.Unlock()

	filePath := v.FileName()
	glog.V(3).Infof("creating copies for volume %d ...", v.Id)
	return v.copyDataAndGenerateIndexFile(filePath+".cpd", filePath+".cpx")
}
func (v *Volume) commitCompact() error {
	v.accessLock.Lock()
	defer v.accessLock.Unlock()
	_ = v.dataFile.Close()
	var e error
	if e = os.Rename(v.FileName()+".cpd", v.FileName()+".dat"); e != nil {
		return e
	}
	if e = os.Rename(v.FileName()+".cpx", v.FileName()+".idx"); e != nil {
		return e
	}
	if e = v.load(true, false); e != nil {
		return e
	}
	return nil
}

func (v *Volume) copyDataAndGenerateIndexFile(dstName, idxName string) (err error) {
	var (
		dst, idx *os.File
	)
	if dst, err = os.OpenFile(dstName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return
	}
	defer dst.Close()

	if idx, err = os.OpenFile(idxName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return
	}
	defer idx.Close()

	nm := NewNeedleMap(idx)
	new_offset := int64(SuperBlockSize)

	err = ScanVolumeFile(v.dir, v.Collection, v.Id, func(superBlock SuperBlock) error {
		_, err = dst.Write(superBlock.Bytes())
		return err
	}, func(n *Needle, offset int64) error {
		nv, ok := v.nm.Get(n.Id)
		glog.V(3).Infoln("needle expected offset ", offset, "ok", ok, "nv", nv)
		if ok && int64(nv.Offset)*NeedlePaddingSize == offset && nv.Size > 0 {
			if _, err = nm.Put(n.Id, uint32(new_offset/NeedlePaddingSize), n.Size); err != nil {
				return fmt.Errorf("cannot put needle: %s", err)
			}
			if _, err = n.Append(dst, v.Version()); err != nil {
				return fmt.Errorf("cannot append needle: %s", err)
			}
			new_offset += n.DiskSize()
			glog.V(3).Infoln("saving key", n.Id, "volume offset", offset, "=>", new_offset, "data_size", n.Size)
		}
		return nil
	})

	return
}