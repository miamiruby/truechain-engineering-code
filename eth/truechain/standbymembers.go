/*
Copyright (c) 2018 TrueChain Foundation
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package truechain

import (
	"bytes"
	"strconv"
	"crypto/ecdsa"
	"math/big"
	"sort"
    "errors"
    
    //"github.com/ethereum/go-ethereum/core/types"
	//"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/core"
)

var blockNum int64 =12
func (t *TrueHybrid) ReceiveSdmMsg(msg *CdEncryptionMsg) {
	t.cdRecv <- msg
}
func (t *TrueHybrid) Vote(num int) ([]*CommitteeMember,error) {
	t.vote <- num
	select {
	case res := <-t.voteRes:
		return res.cmm,res.err
	case <-t.quit:
	}
	return nil,errors.New("vote failed")
}
func (t *TrueHybrid) RemoveFromCommittee(cmm *PbftCommittee) {
	t.removeCd <- cmm
}
///////////////////////////////////////////////////////////////
func (t *TrueHybrid) add(msg *CdEncryptionMsg) error {
	node := msg.ToStandbyInfo()
	if node == nil {
		return errors.New("Wrong CrytoMsg")
	}
	// verfiy and add 
    if len(t.Cdm.Cm) >= t.Sdmsize {
		t.Cdm.Cm = append(t.Cdm.Cm[:0],t.Cdm.Cm[1:]...)
	} 	
	t.Cdm.Cm = append(t.Cdm.Cm,node)
	return nil
}
func (t *TrueHybrid) findMsg(height *big.Int) *CdEncryptionMsg {
	for _,v := range t.Cdm.VCdCrypMsg {
		if v.Height.Cmp(height) == 0 {
			return v
		}
	}
	return nil
}
// check the crypmsg when blockchain has the block
func (t *TrueHybrid) checkTmpMsg() {
	for {
		if len(t.Cdm.NCdCrypMsg) <= 0 {
			break
		}
		msg,pos := minMsg(t.Cdm.NCdCrypMsg,true)
		res := verityMsg(msg,t.bc)
		if res == 1 {
			t.Cdm.VCdCrypMsg = append(t.Cdm.VCdCrypMsg,msg)
			t.Cdm.NCdCrypMsg = t.removemgs(t.Cdm.NCdCrypMsg,pos)
		} else {
			break
		}	
	}
	return
}
// crpmsg was be check and insert to the standbyqueue
// when the blockchain has the block.
func (t *TrueHybrid) insertToSDM() error {
	m,_ := minMsg(t.Cdm.VCdCrypMsg,false)
	if m == nil {
		return errors.New("no minMsg,msglen=" + strconv.Itoa(len(t.Cdm.VCdCrypMsg)))
	}
	msgHeight := m.Height
	cur := big.NewInt(t.bc.CurrentHeader().Number.Int64())	
	if cur.Abs(msgHeight).Cmp(big.NewInt(blockNum)) >= 0 {
		res := verityMsg(m,t.bc)
		if res == 1 {
			t.add(m)
		}
		m.SetUse(true)
		t.removeUnuseMsg(m.Height)
	}
	return nil
}
// remove the msg that has same height and it was used
func (t *TrueHybrid) removeUnuseMsg(num *big.Int) {
	pos := make([]int,0,0)
	for i,v := range t.Cdm.VCdCrypMsg {
		if v.Height.Cmp(num) == 0 {
			if !v.GetUse() {
				pos = append(pos,i)
			}
		}
	}
	for _,i := range pos {
		t.Cdm.VCdCrypMsg = t.removemgs(t.Cdm.VCdCrypMsg,i)
	}
}
func (t *TrueHybrid) removemgs(msg []*CdEncryptionMsg,i int) []*CdEncryptionMsg {
    return append(msg[:i], msg[i+1:]...)
}

func (t *TrueHybrid) matchCommitteeMembers(comm []*CommitteeMember) []int {
	pos := make([]int,0,0)

	for _,v := range comm {
		i := t.posFromCm(v.Nodeid)
		if i != -1 {
			pos = append(pos,i)
		}
	}
	sort.Ints(pos[:])
	c1 := len(comm)
	c2 := len(pos)
	if c1 != c2 || c1 != (pos[c2-1]-pos[0]+1) {
		return nil
	}
	return pos
}
func (t *TrueHybrid) posFromCm(nid string) int {
	for i,v := range t.Cdm.Cm {
		if v.Nodeid == nid {
			return i
		}
	}
	return -1
}
func (t *TrueHybrid) worker() {
	for {
		select{
		case <-t.cdCheck.C:
			t.insertToSDM()
			t.checkTmpMsg()
		case <-t.cdSync.C:
			t.syncStandbyMembers()
			t.SyncMainMembers()
		case n:=<-t.vote:
			res,err := t.voteFromCd(n)
			t.voteRes<-&VoteResult{
				err:	err,
				cmm:	res,
			}
		case msg:=<-t.cdRecv:
			t.handleReceiveSdmMsg(msg)
		case cmm :=<-t.removeCd:
			t.handleRemoveFromCommittee(cmm)
		case <-t.quit:
			return
		}
	}
}
func (t *TrueHybrid) voteFromCd(num int) ([]*CommitteeMember,error) {
	vv := make([]*CommitteeMember,0,0)
	i := 0
	for _,v := range t.Cdm.Cm {
		if i >= num {
			break
		} else {
			vv = append(vv,&CommitteeMember{
				Nodeid:		v.Nodeid,
				Addr:		v.Addr,			
				Port:		v.Port,			
			})
			i++
		}
	}
	return vv,nil
}
func (t *TrueHybrid) syncStandbyMembers() {
	// sync crypmsg
	CdsCh <-t.Cdm.VCdCrypMsg
}
func (t *TrueHybrid) handleReceiveSdmMsg(msg *CdEncryptionMsg) {
	if msg == nil {
		return
	}
	m,_ := minMsg(t.Cdm.VCdCrypMsg,true)
	if m != nil {
		if m.Height.Cmp(msg.Height) <= 0 {
			return 
		}
	}
	if existMsg(msg,t.Cdm.VCdCrypMsg){
		return 
	}
	// verify the msg when the block is on
	res := verityMsg(msg,t.bc)
	if res == 1 {
		t.Cdm.VCdCrypMsg = append(t.Cdm.VCdCrypMsg,msg)
	} else if res == 0 {
		t.Cdm.NCdCrypMsg = append(t.Cdm.NCdCrypMsg,msg)
		if len(t.Cdm.NCdCrypMsg ) > 1000 {
			t.Cdm.NCdCrypMsg = t.removemgs(t.Cdm.NCdCrypMsg, 0)
		}
	}
}
func (t *TrueHybrid) handleRemoveFromCommittee(cmm *PbftCommittee){
	// match the committee number 
	// simple remove(one by one)....
	pos := t.matchCommitteeMembers(cmm.GetCmm())
	if pos != nil {
		for i := len(pos) -1; i > -1; i-- {
			t.Cdm.Cm = append(t.Cdm.Cm[:pos[i]], t.Cdm.Cm[pos[i]+1:]...)
		}
		// update the committee number
	} else {
		// the sdm was dirty,must be update
	}
}

////////////////////////////////////////////////////////////////////////
// use=true include msg which was used
func minMsg(crpmsg []*CdEncryptionMsg,use bool) (*CdEncryptionMsg,int) {
	if len(crpmsg) <= 0 {
		return nil,0
	}
	min := crpmsg[0].Height
	pos := 0
	for ii,v := range crpmsg {
		if use {
			if min.Cmp(v.Height) == 1 {
				min = v.Height
				pos = ii
			}
		} else {
			if crpmsg[pos].GetUse() == true {
				min = v.Height
				pos = ii
			}
			if min.Cmp(v.Height) == -1 {
				min = v.Height
				pos = ii
			}
		}
	}
	if use {
		return crpmsg[pos],pos
	} else {
		if crpmsg[pos].GetUse() {
			return nil,0
		} else {
			return crpmsg[pos],pos
		}
	}
}
func existMsg(msg *CdEncryptionMsg,msgs []*CdEncryptionMsg) bool {
	for _,v := range msgs {
		if v.Height.Cmp(msg.Height) != 0{
			continue
		}
		if len(msg.Msg) != len(v.Msg) || len(msg.Sig) != len(v.Sig) {
			continue
		}
		if bytes.Compare(msg.Msg,v.Msg) == 0 && bytes.Compare(msg.Sig,v.Sig) == 0{
			return true
		}
	}
	return false
}
// after success pow,send the node by p2p
func MakeSignedStandbyNode(n *CdMember,priv *ecdsa.PrivateKey) (*CdEncryptionMsg,error) {
	cmsg := CdEncryptionMsg{
		Height:		n.Height,
		Msg:		make([]byte,0,0),
		Sig:		make([]byte,0,0),
		Use:		false,
	}
	var err error
	cmsg.Msg,err = n.ToByte()
	if err != nil {
		return nil,err
	}
	hash := cmsg.Msg[:32]
	cmsg.Sig,err = crypto.Sign(hash,priv)
	if err != nil {
		return nil,err
	}
	return &cmsg,nil
}
// 0 -- not ready; 1 -- success; -1 -- fail
func verityMsg(msg *CdEncryptionMsg,bc *core.BlockChain) int {
	if msg.Sig == nil || msg.Msg == nil || msg.Height.Cmp(zero) <= 0 {
		return -1
	}
	if bc == nil {
		return 0
	}
	// find the coinbase address from the heigth
	header := bc.GetHeaderByNumber(msg.Height.Uint64())
	if header == nil {
		return 0
	}
	coinbase := header.Coinbase.String()
	hash := msg.Msg[:32]
	pub,err := crypto.SigToPub(hash,msg.Sig)
	if err != nil {
		return -1
	}
	addr := crypto.PubkeyToAddress(*pub).String()
	if addr == coinbase {
		return 1
	}
	return -1
}
