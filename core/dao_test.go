package core

import (
	"math/big"
	"testing"

	"github.com/quickchain/quickchain/consensus/qcthash"
	"github.com/quickchain/quickchain/core/vm"
	"github.com/quickchain/quickchain/qctdb"
	"github.com/quickchain/quickchain/params"
)

// Tests that DAO-fork enabled clients can properly filter out fork-commencing
// blocks based on their extradata fields.
func TestDAOForkRangeExtradata(t *testing.T) {
	forkBlock := big.NewInt(32)

	// Generate a common prefix for both pro-forkers and non-forkers
	db, _ := qctdb.NewMemDatabase()
	gspec := new(Genesis)
	genesis := gspec.MustCommit(db)
	prefix, _ := GenerateChain(params.TestChainConfig, genesis, qcthash.NewFaker(), db, int(forkBlock.Int64()-1), func(i int, gen *BlockGen) {})

	// Create the concurrent, conflicting two nodes
	proDb, _ := qctdb.NewMemDatabase()
	gspec.MustCommit(proDb)

	proConf := *params.TestChainConfig
	proConf.DAOForkBlock = forkBlock
	proConf.DAOForkSupport = true

	proBc, _ := NewBlockChain(proDb, nil, &proConf, qcthash.NewFaker(), vm.Config{})
	defer proBc.Stop()

	conDb, _ := qctdb.NewMemDatabase()
	gspec.MustCommit(conDb)

	conConf := *params.TestChainConfig
	conConf.DAOForkBlock = forkBlock
	conConf.DAOForkSupport = false

	conBc, _ := NewBlockChain(conDb, nil, &conConf, qcthash.NewFaker(), vm.Config{})
	defer conBc.Stop()

	if _, err := proBc.InsertChain(prefix); err != nil {
		t.Fatalf("pro-fork: failed to import chain prefix: %v", err)
	}
	if _, err := conBc.InsertChain(prefix); err != nil {
		t.Fatalf("con-fork: failed to import chain prefix: %v", err)
	}
	// Try to expand both pro-fork and non-fork chains iteratively with other camp's blocks
	for i := int64(0); i < params.DAOForkExtraRange.Int64(); i++ {
		// Create a pro-fork block, and try to feed into the no-fork chain
		db, _ = qctdb.NewMemDatabase()
		gspec.MustCommit(db)
		bc, _ := NewBlockChain(db, nil, &conConf, qcthash.NewFaker(), vm.Config{})
		defer bc.Stop()

		blocks := conBc.GetBlocksFromHash(conBc.CurrentBlock().Hash(), int(conBc.CurrentBlock().NumberU64()))
		for j := 0; j < len(blocks)/2; j++ {
			blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
		}
		if _, err := bc.InsertChain(blocks); err != nil {
			t.Fatalf("failed to import contra-fork chain for expansion: %v", err)
		}
		if err := bc.stateCache.TrieDB().Commit(bc.CurrentHeader().Root, true); err != nil {
			t.Fatalf("failed to commit contra-fork head for expansion: %v", err)
		}
		blocks, _ = GenerateChain(&proConf, conBc.CurrentBlock(), qcthash.NewFaker(), db, 1, func(i int, gen *BlockGen) {})
		if _, err := conBc.InsertChain(blocks); err == nil {
			t.Fatalf("contra-fork chain accepted pro-fork block: %v", blocks[0])
		}
		// Create a proper no-fork block for the contra-forker
		blocks, _ = GenerateChain(&conConf, conBc.CurrentBlock(), qcthash.NewFaker(), db, 1, func(i int, gen *BlockGen) {})
		if _, err := conBc.InsertChain(blocks); err != nil {
			t.Fatalf("contra-fork chain didn't accepted no-fork block: %v", err)
		}
		// Create a no-fork block, and try to feed into the pro-fork chain
		db, _ = qctdb.NewMemDatabase()
		gspec.MustCommit(db)
		bc, _ = NewBlockChain(db, nil, &proConf, qcthash.NewFaker(), vm.Config{})
		defer bc.Stop()

		blocks = proBc.GetBlocksFromHash(proBc.CurrentBlock().Hash(), int(proBc.CurrentBlock().NumberU64()))
		for j := 0; j < len(blocks)/2; j++ {
			blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
		}
		if _, err := bc.InsertChain(blocks); err != nil {
			t.Fatalf("failed to import pro-fork chain for expansion: %v", err)
		}
		if err := bc.stateCache.TrieDB().Commit(bc.CurrentHeader().Root, true); err != nil {
			t.Fatalf("failed to commit pro-fork head for expansion: %v", err)
		}
		blocks, _ = GenerateChain(&conConf, proBc.CurrentBlock(), qcthash.NewFaker(), db, 1, func(i int, gen *BlockGen) {})
		if _, err := proBc.InsertChain(blocks); err == nil {
			t.Fatalf("pro-fork chain accepted contra-fork block: %v", blocks[0])
		}
		// Create a proper pro-fork block for the pro-forker
		blocks, _ = GenerateChain(&proConf, proBc.CurrentBlock(), qcthash.NewFaker(), db, 1, func(i int, gen *BlockGen) {})
		if _, err := proBc.InsertChain(blocks); err != nil {
			t.Fatalf("pro-fork chain didn't accepted pro-fork block: %v", err)
		}
	}
	// Verify that contra-forkers accept pro-fork extra-datas after forking finishes
	db, _ = qctdb.NewMemDatabase()
	gspec.MustCommit(db)
	bc, _ := NewBlockChain(db, nil, &conConf, qcthash.NewFaker(), vm.Config{})
	defer bc.Stop()

	blocks := conBc.GetBlocksFromHash(conBc.CurrentBlock().Hash(), int(conBc.CurrentBlock().NumberU64()))
	for j := 0; j < len(blocks)/2; j++ {
		blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
	}
	if _, err := bc.InsertChain(blocks); err != nil {
		t.Fatalf("failed to import contra-fork chain for expansion: %v", err)
	}
	if err := bc.stateCache.TrieDB().Commit(bc.CurrentHeader().Root, true); err != nil {
		t.Fatalf("failed to commit contra-fork head for expansion: %v", err)
	}
	blocks, _ = GenerateChain(&proConf, conBc.CurrentBlock(), qcthash.NewFaker(), db, 1, func(i int, gen *BlockGen) {})
	if _, err := conBc.InsertChain(blocks); err != nil {
		t.Fatalf("contra-fork chain didn't accept pro-fork block post-fork: %v", err)
	}
	// Verify that pro-forkers accept contra-fork extra-datas after forking finishes
	db, _ = qctdb.NewMemDatabase()
	gspec.MustCommit(db)
	bc, _ = NewBlockChain(db, nil, &proConf, qcthash.NewFaker(), vm.Config{})
	defer bc.Stop()

	blocks = proBc.GetBlocksFromHash(proBc.CurrentBlock().Hash(), int(proBc.CurrentBlock().NumberU64()))
	for j := 0; j < len(blocks)/2; j++ {
		blocks[j], blocks[len(blocks)-1-j] = blocks[len(blocks)-1-j], blocks[j]
	}
	if _, err := bc.InsertChain(blocks); err != nil {
		t.Fatalf("failed to import pro-fork chain for expansion: %v", err)
	}
	if err := bc.stateCache.TrieDB().Commit(bc.CurrentHeader().Root, true); err != nil {
		t.Fatalf("failed to commit pro-fork head for expansion: %v", err)
	}
	blocks, _ = GenerateChain(&conConf, proBc.CurrentBlock(), qcthash.NewFaker(), db, 1, func(i int, gen *BlockGen) {})
	if _, err := proBc.InsertChain(blocks); err != nil {
		t.Fatalf("pro-fork chain didn't accept contra-fork block post-fork: %v", err)
	}
}
