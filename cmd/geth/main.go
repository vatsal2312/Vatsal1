// Copyright 2014 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

// geth is the official command-line client for Ethereum.
package main

import (
	"fmt"
	"math"
	"os"
	godebug "runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/console/prompt"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethofs"
	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/internal/flags"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/node"
	gopsutil "github.com/shirou/gopsutil/mem"
	cli "gopkg.in/urfave/cli.v1"
)

const (
	clientIdentifier = "geth" // Client identifier to advertise over the network
)

var (
	// Git SHA1 commit hash of the release (set via linker flags)
	gitCommit = ""
	gitDate   = ""
	// The app that holds all commands and flags.
	app = flags.NewApp(gitCommit, gitDate, "the go-ethereum command line interface")
	// flags that configure the node
	nodeFlags = []cli.Flag{
		utils.IdentityFlag,
		utils.UnlockedAccountFlag,
		utils.PasswordFileFlag,
		utils.BootnodesFlag,
		utils.LegacyBootnodesV4Flag,
		utils.LegacyBootnodesV5Flag,
		utils.DataDirFlag,
		utils.AncientFlag,
		utils.KeyStoreDirFlag,
		utils.ExternalSignerFlag,
		utils.NoUSBFlag,
		utils.SmartCardDaemonPathFlag,
		utils.EthashCacheDirFlag,
		utils.EthashCachesInMemoryFlag,
		utils.EthashCachesOnDiskFlag,
		utils.EthashCachesLockMmapFlag,
		utils.EthashDatasetDirFlag,
		utils.EthashDatasetsInMemoryFlag,
		utils.EthashDatasetsOnDiskFlag,
		utils.EthofsFlag,
		utils.EthofsConfigFlag,
		utils.EthofsInitFlag,
		utils.EthashDatasetsLockMmapFlag,
		utils.TxPoolLocalsFlag,
		utils.TxPoolNoLocalsFlag,
		utils.TxPoolJournalFlag,
		utils.TxPoolRejournalFlag,
		utils.TxPoolPriceLimitFlag,
		utils.TxPoolPriceBumpFlag,
		utils.TxPoolAccountSlotsFlag,
		utils.TxPoolGlobalSlotsFlag,
		utils.TxPoolAccountQueueFlag,
		utils.TxPoolGlobalQueueFlag,
		utils.TxPoolLifetimeFlag,
		utils.SyncModeFlag,
		utils.ExitWhenSyncedFlag,
		utils.GCModeFlag,
		utils.SnapshotFlag,
		utils.TxLookupLimitFlag,
		utils.LightServeFlag,
		utils.LegacyLightServFlag,
		utils.LightIngressFlag,
		utils.LightEgressFlag,
		utils.LightMaxPeersFlag,
		utils.LegacyLightPeersFlag,
		utils.LightNoPruneFlag,
		utils.LightKDFFlag,
		utils.UltraLightServersFlag,
		utils.UltraLightFractionFlag,
		utils.UltraLightOnlyAnnounceFlag,
		utils.WhitelistFlag,
		utils.CacheFlag,
		utils.CacheDatabaseFlag,
		utils.CacheTrieFlag,
		utils.CacheTrieJournalFlag,
		utils.CacheTrieRejournalFlag,
		utils.CacheGCFlag,
		utils.CacheSnapshotFlag,
		utils.CacheNoPrefetchFlag,
		utils.ListenPortFlag,
		utils.MaxPeersFlag,
		utils.MaxPendingPeersFlag,
		utils.MiningEnabledFlag,
		utils.MinerThreadsFlag,
		utils.LegacyMinerThreadsFlag,
		utils.MinerNotifyFlag,
		utils.MinerGasTargetFlag,
		utils.LegacyMinerGasTargetFlag,
		utils.MinerGasLimitFlag,
		utils.MinerGasPriceFlag,
		utils.LegacyMinerGasPriceFlag,
		utils.MinerEtherbaseFlag,
		utils.LegacyMinerEtherbaseFlag,
		utils.MinerExtraDataFlag,
		utils.LegacyMinerExtraDataFlag,
		utils.MinerRecommitIntervalFlag,
		utils.MinerNoVerfiyFlag,
		utils.NATFlag,
		utils.NoDiscoverFlag,
		utils.DiscoveryV5Flag,
		utils.NetrestrictFlag,
		utils.NodeKeyFileFlag,
		utils.NodeKeyHexFlag,
		utils.DNSDiscoveryFlag,
		utils.DeveloperFlag,
		utils.DeveloperPeriodFlag,
		utils.LegacyTestnetFlag,
		utils.RopstenFlag,
		utils.RinkebyFlag,
		utils.GoerliFlag,
		utils.YoloV1Flag,
		utils.VMEnableDebugFlag,
		utils.NetworkIdFlag,
		utils.EthStatsURLFlag,
		utils.FakePoWFlag,
		utils.NoCompactionFlag,
		utils.GpoBlocksFlag,
		utils.LegacyGpoBlocksFlag,
		utils.GpoPercentileFlag,
		utils.LegacyGpoPercentileFlag,
		utils.GpoMaxGasPriceFlag,
		utils.EWASMInterpreterFlag,
		utils.EVMInterpreterFlag,
		configFileFlag,
	}

	rpcFlags = []cli.Flag{
		utils.HTTPEnabledFlag,
		utils.HTTPListenAddrFlag,
		utils.HTTPPortFlag,
		utils.HTTPCORSDomainFlag,
		utils.HTTPVirtualHostsFlag,
		utils.LegacyRPCEnabledFlag,
		utils.LegacyRPCListenAddrFlag,
		utils.LegacyRPCPortFlag,
		utils.LegacyRPCCORSDomainFlag,
		utils.LegacyRPCVirtualHostsFlag,
		utils.GraphQLEnabledFlag,
		utils.GraphQLCORSDomainFlag,
		utils.GraphQLVirtualHostsFlag,
		utils.HTTPApiFlag,
		utils.LegacyRPCApiFlag,
		utils.WSEnabledFlag,
		utils.WSListenAddrFlag,
		utils.LegacyWSListenAddrFlag,
		utils.WSPortFlag,
		utils.LegacyWSPortFlag,
		utils.WSApiFlag,
		utils.LegacyWSApiFlag,
		utils.WSAllowedOriginsFlag,
		utils.LegacyWSAllowedOriginsFlag,
		utils.IPCDisabledFlag,
		utils.IPCPathFlag,
		utils.InsecureUnlockAllowedFlag,
		utils.RPCGlobalGasCap,
		utils.RPCGlobalTxFeeCap,
	}

	whisperFlags = []cli.Flag{
		utils.WhisperEnabledFlag,
		utils.WhisperMaxMessageSizeFlag,
		utils.WhisperMinPOWFlag,
		utils.WhisperRestrictConnectionBetweenLightClientsFlag,
	}

	metricsFlags = []cli.Flag{
		utils.MetricsEnabledFlag,
		utils.MetricsEnabledExpensiveFlag,
		utils.MetricsHTTPFlag,
		utils.MetricsPortFlag,
		utils.MetricsEnableInfluxDBFlag,
		utils.MetricsInfluxDBEndpointFlag,
		utils.MetricsInfluxDBDatabaseFlag,
		utils.MetricsInfluxDBUsernameFlag,
		utils.MetricsInfluxDBPasswordFlag,
		utils.MetricsInfluxDBTagsFlag,
	}
)

func init() {
	// Initialize the CLI app and start Geth
	app.Action = geth
	app.HideVersion = true // we have a command to print the version
	app.Copyright = "Copyright 2013-2020 The go-ethereum Authors"
	app.Commands = []cli.Command{
		// See chaincmd.go:
		initCommand,
		importCommand,
		exportCommand,
		importPreimagesCommand,
		exportPreimagesCommand,
		copydbCommand,
		removedbCommand,
		dumpCommand,
		dumpGenesisCommand,
		inspectCommand,
		// See accountcmd.go:
		accountCommand,
		walletCommand,
		// See consolecmd.go:
		consoleCommand,
		attachCommand,
		javascriptCommand,
		// See misccmd.go:
		makecacheCommand,
		makedagCommand,
		versionCommand,
		licenseCommand,
		// See config.go
		dumpConfigCommand,
		// See retesteth.go
		retestethCommand,
		// See cmd/utils/flags_legacy.go
		utils.ShowDeprecated,
	}
	sort.Sort(cli.CommandsByName(app.Commands))

	app.Flags = append(app.Flags, nodeFlags...)
	app.Flags = append(app.Flags, rpcFlags...)
	app.Flags = append(app.Flags, consoleFlags...)
	app.Flags = append(app.Flags, debug.Flags...)
	app.Flags = append(app.Flags, debug.DeprecatedFlags...)
	app.Flags = append(app.Flags, whisperFlags...)
	app.Flags = append(app.Flags, metricsFlags...)

	app.Before = func(ctx *cli.Context) error {
		return debug.Setup(ctx)
	}
	app.After = func(ctx *cli.Context) error {
		debug.Exit()
		prompt.Stdin.Close() // Resets terminal mode.
		return nil
	}
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// prepare manipulates memory cache allowance and setups metric system.
// This function should be called before launching devp2p stack.
func prepare(ctx *cli.Context) {
	// If we're running a known preset, log it for convenience.
	switch {
	case ctx.GlobalIsSet(utils.LegacyTestnetFlag.Name):
		log.Info("Starting Geth on Ropsten testnet...")
		log.Warn("The --testnet flag is ambiguous! Please specify one of --goerli, --rinkeby, or --ropsten.")
		log.Warn("The generic --testnet flag is deprecated and will be removed in the future!")

	case ctx.GlobalIsSet(utils.RopstenFlag.Name):
		log.Info("Starting Geth on Ropsten testnet...")

	case ctx.GlobalIsSet(utils.RinkebyFlag.Name):
		log.Info("Starting Geth on Rinkeby testnet...")

	case ctx.GlobalIsSet(utils.GoerliFlag.Name):
		log.Info("Starting Geth on Görli testnet...")

	case ctx.GlobalIsSet(utils.DeveloperFlag.Name):
		log.Info("Starting Geth in ephemeral dev mode...")

	case !ctx.GlobalIsSet(utils.NetworkIdFlag.Name):
		log.Info("Starting Geth on Ethereum mainnet...")
	}
	// If we're a full node on mainnet without --cache specified, bump default cache allowance
	if ctx.GlobalString(utils.SyncModeFlag.Name) != "light" && !ctx.GlobalIsSet(utils.CacheFlag.Name) && !ctx.GlobalIsSet(utils.NetworkIdFlag.Name) {
		// Make sure we're not on any supported preconfigured testnet either
		if !ctx.GlobalIsSet(utils.LegacyTestnetFlag.Name) && !ctx.GlobalIsSet(utils.RopstenFlag.Name) && !ctx.GlobalIsSet(utils.RinkebyFlag.Name) && !ctx.GlobalIsSet(utils.GoerliFlag.Name) && !ctx.GlobalIsSet(utils.DeveloperFlag.Name) {
			// Nope, we're really on mainnet. Bump that cache up!
			log.Info("Bumping default cache on mainnet", "provided", ctx.GlobalInt(utils.CacheFlag.Name), "updated", 4096)
			ctx.GlobalSet(utils.CacheFlag.Name, strconv.Itoa(4096))
		}
	}
	// If we're running a light client on any network, drop the cache to some meaningfully low amount
	if ctx.GlobalString(utils.SyncModeFlag.Name) == "light" && !ctx.GlobalIsSet(utils.CacheFlag.Name) {
		log.Info("Dropping default light client cache", "provided", ctx.GlobalInt(utils.CacheFlag.Name), "updated", 128)
		ctx.GlobalSet(utils.CacheFlag.Name, strconv.Itoa(128))
	}
	// Cap the cache allowance and tune the garbage collector
	mem, err := gopsutil.VirtualMemory()
	if err == nil {
		if 32<<(^uintptr(0)>>63) == 32 && mem.Total > 2*1024*1024*1024 {
			log.Warn("Lowering memory allowance on 32bit arch", "available", mem.Total/1024/1024, "addressable", 2*1024)
			mem.Total = 2 * 1024 * 1024 * 1024
		}
		allowance := int(mem.Total / 1024 / 1024 / 3)
		if cache := ctx.GlobalInt(utils.CacheFlag.Name); cache > allowance {
			log.Warn("Sanitizing cache to Go's GC limits", "provided", cache, "updated", allowance)
			ctx.GlobalSet(utils.CacheFlag.Name, strconv.Itoa(allowance))
		}
	}
	// Ensure Go's GC ignores the database cache for trigger percentage
	cache := ctx.GlobalInt(utils.CacheFlag.Name)
	gogc := math.Max(20, math.Min(100, 100/(float64(cache)/1024)))

	log.Debug("Sanitizing Go's GC trigger", "percent", int(gogc))
	godebug.SetGCPercent(int(gogc))

	// Start metrics export if enabled
	utils.SetupMetrics(ctx)

	// Start system runtime metrics collection
	go metrics.CollectProcessMetrics(3 * time.Second)
}

// geth is the main entry point into the system if no special subcommand is ran.
// It creates a default node based on the command line arguments and runs it in
// blocking mode, waiting for it to be shut down.
func geth(ctx *cli.Context) error {
	if args := ctx.Args(); len(args) > 0 {
		return fmt.Errorf("invalid command: %q", args[0])
	}

	prepare(ctx)
	stack, backend := makeFullNode(ctx)
	defer stack.Close()

	startNode(ctx, stack, backend)
	stack.Wait()
	return nil
}

// startNode boots up the system node and all registered protocols, after which
// it unlocks any requested accounts, and starts the RPC/IPC interfaces and the
// miner.
func startNode(ctx *cli.Context, stack *node.Node, backend ethapi.Backend) {
	debug.Memsize.Add("node", stack)

	// Start up the node itself
	utils.StartNode(stack)

	// Unlock any account specifically requested
	unlockAccounts(ctx, stack)

	// Register wallet event handlers to open and auto-derive wallets
	events := make(chan accounts.WalletEvent, 16)
	stack.AccountManager().Subscribe(events)

	// Create a client to interact with local geth node.
	rpcClient, err := stack.Attach()
	if err != nil {
		utils.Fatalf("Failed to attach to self: %v", err)
	}
	ethClient := ethclient.NewClient(rpcClient)

	// Check for ethoFS enabled node and initalize accordingly
	if ctx.GlobalIsSet(utils.EthofsFlag.Name) && (ctx.GlobalString(utils.EthofsFlag.Name) == "gn" || ctx.GlobalString(utils.EthofsFlag.Name) == "mn" || ctx.GlobalString(utils.EthofsFlag.Name) == "sn") {
		if ctx.GlobalBool(utils.EthofsInitFlag.Name) {
			blockCommunication := make(chan *types.Block)
			ethofs.InitializeEthofs(ctx.GlobalBool(utils.EthofsInitFlag.Name), ctx.GlobalBool(utils.EthofsConfigFlag.Name), ctx.GlobalString(utils.EthofsFlag.Name), blockCommunication)
		} else if ctx.GlobalBool(utils.EthofsConfigFlag.Name) {
			blockCommunication := make(chan *types.Block)
			ethofs.InitializeEthofs(ctx.GlobalBool(utils.EthofsInitFlag.Name), ctx.GlobalBool(utils.EthofsConfigFlag.Name), ctx.GlobalString(utils.EthofsFlag.Name), blockCommunication)
		} else {
			blockCommunication := make(chan *types.Block)
			core.InitializeBlockCommunication(blockCommunication)
			ethofs.InitializeEthofs(ctx.GlobalBool(utils.EthofsInitFlag.Name), ctx.GlobalBool(utils.EthofsConfigFlag.Name), ctx.GlobalString(utils.EthofsFlag.Name), blockCommunication)
		}
	} else if ctx.GlobalIsSet(utils.EthofsFlag.Name) {
		log.Error("Invalid ethoFS Flag/Node Type - Exiting")
		os.Exit(0)
	}

	go func() {
		// Open any wallets already attached
		for _, wallet := range stack.AccountManager().Wallets() {
			if err := wallet.Open(""); err != nil {
				log.Warn("Failed to open wallet", "url", wallet.URL(), "err", err)
			}
		}
		// Listen for wallet event till termination
		for event := range events {
			switch event.Kind {
			case accounts.WalletArrived:
				if err := event.Wallet.Open(""); err != nil {
					log.Warn("New wallet appeared, failed to open", "url", event.Wallet.URL(), "err", err)
				}
			case accounts.WalletOpened:
				status, _ := event.Wallet.Status()
				log.Info("New wallet appeared", "url", event.Wallet.URL(), "status", status)

				var derivationPaths []accounts.DerivationPath
				if event.Wallet.URL().Scheme == "ledger" {
					derivationPaths = append(derivationPaths, accounts.LegacyLedgerBaseDerivationPath)
				}
				derivationPaths = append(derivationPaths, accounts.DefaultBaseDerivationPath)

				event.Wallet.SelfDerive(derivationPaths, ethClient)

			case accounts.WalletDropped:
				log.Info("Old wallet dropped", "url", event.Wallet.URL())
				event.Wallet.Close()
			}
		}
	}()

	// Spawn a standalone goroutine for status synchronization monitoring,
	// close the node when synchronization is complete if user required.
	if ctx.GlobalBool(utils.ExitWhenSyncedFlag.Name) {
		go func() {
			sub := stack.EventMux().Subscribe(downloader.DoneEvent{})
			defer sub.Unsubscribe()
			for {
				event := <-sub.Chan()
				if event == nil {
					continue
				}
				done, ok := event.Data.(downloader.DoneEvent)
				if !ok {
					continue
				}
				if timestamp := time.Unix(int64(done.Latest.Time), 0); time.Since(timestamp) < 10*time.Minute {
					log.Info("Synchronisation completed", "latestnum", done.Latest.Number, "latesthash", done.Latest.Hash(),
						"age", common.PrettyAge(timestamp))
					stack.Close()
				}
			}
		}()
	}

	// Start auxiliary services if enabled
	if ctx.GlobalBool(utils.MiningEnabledFlag.Name) || ctx.GlobalBool(utils.DeveloperFlag.Name) {
		// Mining only makes sense if a full Ethereum node is running
		if ctx.GlobalString(utils.SyncModeFlag.Name) == "light" {
			utils.Fatalf("Light clients do not support mining")
		}
		ethBackend, ok := backend.(*eth.EthAPIBackend)
		if !ok {
			utils.Fatalf("Ethereum service not running: %v", err)
		}

		// Set the gas price to the limits from the CLI and start mining
		gasprice := utils.GlobalBig(ctx, utils.MinerGasPriceFlag.Name)
		if ctx.GlobalIsSet(utils.LegacyMinerGasPriceFlag.Name) && !ctx.GlobalIsSet(utils.MinerGasPriceFlag.Name) {
			gasprice = utils.GlobalBig(ctx, utils.LegacyMinerGasPriceFlag.Name)
		}
		ethBackend.TxPool().SetGasPrice(gasprice)
		// start mining
		threads := ctx.GlobalInt(utils.MinerThreadsFlag.Name)
		if ctx.GlobalIsSet(utils.LegacyMinerThreadsFlag.Name) && !ctx.GlobalIsSet(utils.MinerThreadsFlag.Name) {
			threads = ctx.GlobalInt(utils.LegacyMinerThreadsFlag.Name)
			log.Warn("The flag --minerthreads is deprecated and will be removed in the future, please use --miner.threads")
		}
		if err := ethBackend.StartMining(threads); err != nil {
			utils.Fatalf("Failed to start mining: %v", err)
		}
	}
}

// unlockAccounts unlocks any account specifically requested.
func unlockAccounts(ctx *cli.Context, stack *node.Node) {
	var unlocks []string
	inputs := strings.Split(ctx.GlobalString(utils.UnlockedAccountFlag.Name), ",")
	for _, input := range inputs {
		if trimmed := strings.TrimSpace(input); trimmed != "" {
			unlocks = append(unlocks, trimmed)
		}
	}
	// Short circuit if there is no account to unlock.
	if len(unlocks) == 0 {
		return
	}
	// If insecure account unlocking is not allowed if node's APIs are exposed to external.
	// Print warning log to user and skip unlocking.
	if !stack.Config().InsecureUnlockAllowed && stack.Config().ExtRPCEnabled() {
		utils.Fatalf("Account unlock with HTTP access is forbidden!")
	}
	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	passwords := utils.MakePasswordList(ctx)
	for i, account := range unlocks {
		unlockAccount(ks, account, i, passwords)
	}
}
