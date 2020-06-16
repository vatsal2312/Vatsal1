package ethofs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"strings"

	config "github.com/ipfs/go-ipfs-config"
	files "github.com/ipfs/go-ipfs-files"
	libp2p "github.com/ipfs/go-ipfs/core/node/libp2p"
	icore "github.com/ipfs/interface-go-ipfs-core"
	peerstore "github.com/libp2p/go-libp2p-peerstore"
	ma "github.com/multiformats/go-multiaddr"

	assets "github.com/ipfs/go-ipfs/assets"
	namesys "github.com/ipfs/go-ipfs/namesys"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"

	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/coreapi"
	// This package is needed so that all the preloaded plugins are loaded automatically
	"github.com/ipfs/go-ipfs/plugin/loader"
	"github.com/ipfs/go-ipfs/repo/fsrepo"
	"github.com/libp2p/go-libp2p-core/peer"
)

var Api icore.CoreAPI

const (
	nBitsForKeypairDefault = 2048
	bitsOptionName         = "bits"
	emptyRepoOptionName    = "empty-repo"
	profileOptionName      = "profile"
)

var errRepoExists = errors.New(`ipfs configuration file already exists!
Reinitializing would overwrite your keys.
`)

// Setting up the ethoFS/IPFS Repo
func setupPlugins(externalPluginsPath string) error {
	// Load any external plugins if available on externalPluginsPath
	plugins, err := loader.NewPluginLoader(filepath.Join(externalPluginsPath, "plugins"))
	if err != nil {
		return fmt.Errorf("error loading plugins: %s", err)
	}

	// Load preloaded and external plugins
	if err := plugins.Initialize(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	if err := plugins.Inject(); err != nil {
		return fmt.Errorf("error injecting plugins: %s", err)
	}

	return nil
}

func createTempRepo(ctx context.Context) (string, error) {
	repoPath, err := ioutil.TempDir("", "ipfs-shell")
	if err != nil {
		return "", fmt.Errorf("failed to get temp dir: %s", err)
	}

	// Create a config with default options and a 2048 bit key
	cfg, err := config.Init(ioutil.Discard, 2048)
	if err != nil {
		return "", err
	}

	// Create the repo with the config
	err = fsrepo.Init(repoPath, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to init ephemeral node: %s", err)
	}

	return repoPath, nil
}

func swarmPeers(ctx context.Context) {
		conns, err := Api.Swarm().Peers(ctx)
		if err != nil {
			log.Error("Error swarming ethoFS peers")
		}

		for _, c := range conns {
			addr := c.Address().String()
			peer := c.ID().Pretty()
			log.Info("ethoFS peer connection found", "addr", addr, "id", peer) 
		}

}

// Creates an ethoFS/IPFS node and returns its coreAPI
func createNode(ctx context.Context, repoPath string) (icore.CoreAPI, error) {
	// Open the repo
	repo, err := fsrepo.Open(repoPath)
	if err != nil {
		return nil, err
	}

	// Construct the node

	nodeOptions := &core.BuildCfg{
		Online:  true,
		// This option sets the node to be a full DHT node (both fetching and storing DHT Records)
		Routing: libp2p.DHTOption,
		// This option sets the node to be a client DHT node (only fetching records)
		// Routing: libp2p.DHTClientOption,
		Repo: repo,
	}

	node, err := core.NewNode(ctx, nodeOptions)
	if err != nil {
		return nil, err
	}

	Node = node // Assign node to stored ethoFS node var

	// Attach the Core API to the constructed node
	api, apiErr := coreapi.NewCoreAPI(node)
	Api = api
	return api, apiErr
}

// Spawns a node on the default repo location, if the repo exists
func spawnDefault(ctx context.Context) (icore.CoreAPI, error) {
	/*defaultPath, err := config.PathRoot()
	if err != nil {
		// shouldn't be possible
		return nil, err
	}*/


	defaultPath := node.DefaultDataDir() + "/ethofs"

	if err := setupPlugins(defaultPath); err != nil {
		return nil, err

	}

	return createNode(ctx, defaultPath)
}

// Spawns a node to be used just for this run (i.e. creates a tmp repo)
func spawnEphemeral(ctx context.Context) (icore.CoreAPI, error) {
	if err := setupPlugins(""); err != nil {
		return nil, err
	}

	// Create a Temporary Repo
	repoPath, err := createTempRepo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp repo: %s", err)
	}

	// Spawning an ephemeral IPFS node
	return createNode(ctx, repoPath)
}

func connectToPeers(ctx context.Context, ipfs icore.CoreAPI, peers []string) error {
	var wg sync.WaitGroup
	peerInfos := make(map[peer.ID]*peerstore.PeerInfo, len(peers))
	for _, addrStr := range peers {
		addr, err := ma.NewMultiaddr(addrStr)
		if err != nil {
			return err
		}
		pii, err := peerstore.InfoFromP2pAddr(addr)
		if err != nil {
			return err
		}
		pi, ok := peerInfos[pii.ID]
		if !ok {
			pi = &peerstore.PeerInfo{ID: pii.ID}
			peerInfos[pi.ID] = pi
		}
		pi.Addrs = append(pi.Addrs, pii.Addrs...)
	}

	wg.Add(len(peerInfos))
	for _, peerInfo := range peerInfos {
		go func(peerInfo *peerstore.PeerInfo) {
			defer wg.Done()
			err := ipfs.Swarm().Connect(ctx, *peerInfo)
			if err != nil {
				log.Debug("ethoFS peer connection failed", "node", peerInfo.ID, "message", err)
			} else {
				log.Info("ethoFS peer connection successful", "node", peerInfo.ID)
			}
		}(peerInfo)
	}
	wg.Wait()
	return nil
}

func getUnixfsFile(path string) (files.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	st, err := file.Stat()
	if err != nil {
		return nil, err
	}

	f, err := files.NewReaderPathFile(path, file, st)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func getUnixfsNode(path string) (files.Node, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	f, err := files.NewSerialFile(path, false, st)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func applyProfiles(conf *config.Config, profiles string) error {
	if profiles == "" {
		return nil
	}

	for _, profile := range strings.Split(profiles, ",") {
		transformer, ok := config.Profiles[profile]
		if !ok {
			return fmt.Errorf("invalid configuration profile: %s", profile)
		}

		if err := transformer.Transform(conf); err != nil {
			return err
		}
	}
	return nil
}

func doInit(out io.Writer, repoRoot string, empty bool, nBitsForKeypair int, confProfiles string, conf *config.Config) error {
	if _, err := fmt.Fprintf(out, "initializing ethoFS node at %s\n", repoRoot); err != nil {
		return err
	}

	if err := checkWritable(repoRoot); err != nil {
		return err
	}

	if fsrepo.IsInitialized(repoRoot) {
		return errRepoExists
	}

	if conf == nil {
		var err error
		conf, err = config.Init(out, nBitsForKeypair)
		if err != nil {
			return err
		}
	}

	if err := applyProfiles(conf, confProfiles); err != nil {
		return err
	}

	if err := fsrepo.Init(repoRoot, conf); err != nil {
		return err
	}

	if !empty {
		if err := addDefaultAssets(out, repoRoot); err != nil {
			return err
		}
	}


	// Create swarm key for ethoFS private network
	err := createSwarmKey(repoRoot)
	if err != nil {
		log.Error("Error creating ethoFS swarm key", "error", err)
		return err
	}

	return initializeIpnsKeyspace(repoRoot)
}

func createSwarmKey(repoRoot string) error {
	f, err := os.Create(repoRoot + "/swarm.key")
	if err != nil {
		return err
	}
	_, err = f.WriteString("/key/swarm/psk/1.0.0/\n/base16/\n38307a74b2176d0054ffa2864e31ee22d0fc6c3266dd856f6d41bddf14e2ad63")
	if err != nil {
		f.Close()
        	return err
	}

	log.Info("ethoFS swarm key created successfully")
	err = f.Close()
	if err != nil {
		return err
	}

	return nil
}

func checkWritable(dir string) error {
	_, err := os.Stat(dir)
	if err == nil {
		// dir exists, make sure we can write to it
		testfile := filepath.Join(dir, "test")
		fi, err := os.Create(testfile)
		if err != nil {
			if os.IsPermission(err) {
				return fmt.Errorf("%s is not writeable by the current user", dir)
			}
			return fmt.Errorf("unexpected error while checking writeablility of repo root: %s", err)
		}
		fi.Close()
		return os.Remove(testfile)
	}

	if os.IsNotExist(err) {
		// dir doesn't exist, check that we can create it
		return os.Mkdir(dir, 0775)
	}

	if os.IsPermission(err) {
		return fmt.Errorf("cannot write to %s, incorrect permissions", err)
	}

	return err
}

func addDefaultAssets(out io.Writer, repoRoot string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := fsrepo.Open(repoRoot)
	if err != nil { // NB: repo is owned by the node
		return err
	}

	nd, err := core.NewNode(ctx, &core.BuildCfg{Repo: r})
	if err != nil {
		return err
	}
	defer nd.Close()

	dkey, err := assets.SeedInitDocs(nd)
	if err != nil {
		return fmt.Errorf("init: seeding init docs failed: %s", err)
	}
	log.Warn("init: seeded init docs %s", dkey)

	if _, err = fmt.Fprintf(out, "to get started, enter:\n"); err != nil {
		return err
	}

	_, err = fmt.Fprintf(out, "\n\tipfs cat /ipfs/%s/readme\n\n", dkey)
	return err
}

func initializeIpnsKeyspace(repoRoot string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r, err := fsrepo.Open(repoRoot)
	if err != nil { // NB: repo is owned by the node
		return err
	}

	nd, err := core.NewNode(ctx, &core.BuildCfg{Repo: r})
	if err != nil {
		return err
	}
	defer nd.Close()

	return namesys.InitializeKeyspace(ctx, nd.Namesys, nd.Pinning, nd.PrivateKey)
}

func initializeEthofsRepo() error {

	empty := true
	nBitsForKeypair  := nBitsForKeypairDefault

	var conf *config.Config

	/*f := req.Files
	if f != nil {
		it := req.Files.Entries()
		if !it.Next() {
			if it.Err() != nil {
				return it.Err()
			}
			return fmt.Errorf("file argument was nil")
		}
		file := files.FileFromEntry(it)
		if file == nil {
			return fmt.Errorf("expected a regular file")
		}

		if err := json.NewDecoder(file).Decode(conf); err != nil {
			return err
		}
	}*/

	//conf = &config.Config{}
	//profiles := profileOptionName
	profiles := "lowpower"

        //repoPath, _ := config.PathRoot()
	repoPath := node.DefaultDataDir() + "/ethofs"

	//return doInit(os.Stdout, cctx.ConfigRoot, empty, nBitsForKeypair, profiles, conf)
	return doInit(os.Stdout, repoPath, empty, nBitsForKeypair, profiles, conf)
}

func initializeEthofsNode() {

	log.Info("Deploying ethoFS node")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Spawn a node using the default path (~/.ipfs), assuming that a repo exists there already
	log.Info("Initializing ethoFS node on default repo path")
	ipfs, err := spawnDefault(ctx)
	//_, err := spawnDefault(ctx)
	if err != nil {
		log.Warn("Unable to intialize ethoFS node on default repo path", "error", err)
		log.Info("ethoFS node repo initialization started")
		initErr := initializeEthofsRepo()
		if initErr != nil {
			log.Error("Unable to initalize ethoFS repo on default path", "error", initErr)
			os.Exit(0)
		} else {
			log.Info("Retrying ethoFS node depoloyment")
			initializeEthofsNode()
		}
	}

	// Spawn a node using a temporary path, creating a temporary repo for the run
	/*log.Info("Spawning ethoFS node on a temporary repo")
	ipfs, err := spawnEphemeral(ctx)
	if err != nil {
		panic(fmt.Errorf("failed to spawn ephemeral ethoFS node: %s", err))
	}*/

	log.Info("ethoFS node initialization complete")

	//log.Info("Retrieving ethoFS Data")

	/*inputBasePath := "./example-folder/"
	inputPathFile := inputBasePath + "ipfs.paper.draft3.pdf"
	inputPathDirectory := inputBasePath + "test-dir"

	someFile, err := getUnixfsNode(inputPathFile)
	if err != nil {
		panic(fmt.Errorf("Could not get File: %s", err))
	}

	cidFile, err := ipfs.Unixfs().Add(ctx, someFile)
	if err != nil {
		panic(fmt.Errorf("Could not add File: %s", err))
	}

	fmt.Printf("Added file to ethoFS with CID %s\n", cidFile.String())

	someDirectory, err := getUnixfsNode(inputPathDirectory)
	if err != nil {
		panic(fmt.Errorf("Could not get File: %s", err))
	}

	cidDirectory, err := ipfs.Unixfs().Add(ctx, someDirectory)
	if err != nil {
		panic(fmt.Errorf("Could not add Directory: %s", err))
	}

	fmt.Printf("Added directory to ethoFS with CID %s\n", cidDirectory.String())

	outputBasePath := "./example-folder/"
	outputPathFile := outputBasePath + strings.Split(cidFile.String(), "/")[2]
	outputPathDirectory := outputBasePath + strings.Split(cidDirectory.String(), "/")[2]

	rootNodeFile, err := ipfs.Unixfs().Get(ctx, cidFile)
	if err != nil {
		panic(fmt.Errorf("Could not get file with CID: %s", err))
	}

	err = files.WriteTo(rootNodeFile, outputPathFile)
	if err != nil {
		panic(fmt.Errorf("Could not write out the fetched CID: %s", err))
	}

	fmt.Printf("Got file back from IPFS (IPFS path: %s) and wrote it to %s\n", cidFile.String(), outputPathFile)

	rootNodeDirectory, err := ipfs.Unixfs().Get(ctx, cidDirectory)
	if err != nil {
		panic(fmt.Errorf("Could not get file with CID: %s", err))
	}

	err = files.WriteTo(rootNodeDirectory, outputPathDirectory)
	if err != nil {
		panic(fmt.Errorf("Could not write out the fetched CID: %s", err))
	}

	fmt.Printf("Got directory back from IPFS (IPFS path: %s) and wrote it to %s\n", cidDirectory.String(), outputPathDirectory)

	fmt.Println("\n-- Going to connect to a few nodes in the Network as bootstrappers --")
	*/

	bootstrapNodes := []string{
		"/ip4/164.68.107.82/tcp/4001/ipfs/QmeG81bELkgLBZFYZc53ioxtvRS8iNVzPqxUBKSuah2rcQ",
		"/ip4/164.68.98.94/tcp/4001/ipfs/QmRYw68MzD4jPvner913mLWBdFfpPfNUx8SRFjiUCJNA4f",
		"/ip4/51.38.131.241/tcp/4001/ipfs/QmaGGSUqoFpv6wuqvNKNBsxDParVuGgV3n3iPs2eVWeSN4",
		"/ip4/164.68.108.54/tcp/4001/ipfs/QmRwQ49Zknc2dQbywrhT8ArMDS9JdmnEyGGy4mZ1wDkgaX",
		"/ip4/51.77.150.202/tcp/4001/ipfs/QmUEy4ScCYCgP6GRfVgrLDqXfLXnUUh4eKaS1fDgaCoGQJ",
		"/ip4/51.79.70.144/tcp/4001/ipfs/QmTcwcKqKcnt84wCecShm1zdz1KagfVtqopg1xKLiwVJst",
		"/ip4/142.44.246.43/tcp/4001/ipfs/QmPW8zExrEeno85Us3H1bk68rBo7N7WEhdpU9pC9wjQxgu",
	}

	connectToPeers(ctx, ipfs, bootstrapNodes)

	/*exampleCIDStr := "QmUaoioqU7bxezBQZkUcgcSyokatMY71sxsALxQmRRrHrj"

	fmt.Printf("Fetching a file from the network with CID %s\n", exampleCIDStr)
	outputPath := outputBasePath + exampleCIDStr
	testCID := icorepath.New(exampleCIDStr)

	rootNode, err := ipfs.Unixfs().Get(ctx, testCID)
	if err != nil {
		panic(fmt.Errorf("Could not get file with CID: %s", err))
	}

	err = files.WriteTo(rootNode, outputPath)
	if err != nil {
		panic(fmt.Errorf("Could not write out the fetched CID: %s", err))
	}

	fmt.Printf("Wrote the file to %s\n", outputPath)
	*/
}