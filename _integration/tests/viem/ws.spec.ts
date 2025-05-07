import { createPublicClient, http, webSocket} from "viem"
import { mainnet, base } from "@gfxlabs/oku-chains"

const chains = [mainnet, base]

const venn_url = process.env.VENN_URL || `localhost:8545`

describe("ws simple", ()=>{
  const provider = Object.fromEntries(chains.map((x)=>{
    const tp = webSocket(`ws://${venn_url}/${x.internalName}`)
    const pc = createPublicClient({
      chain: x,
      transport:tp,
    })
    afterAll(()=>{
      pc.transport.getRpcClient().then((x)=>{
        x.close()
      })
    })
    return [x.id, pc]
  }))
  test("able to get mainnet block number", async ()=>{
    const blockNumber = await provider[mainnet.id].getBlockNumber()
    expect(blockNumber).toBeGreaterThan(19066961n)
  })
  test("base block changes within 5 seconds", async ()=>{
    const blockNumberOld = await provider[base.id].getBlockNumber()
    await new Promise(resolve => setTimeout(resolve, 5000));
    const blockNumberNew = await provider[base.id].getBlockNumber()
    expect(blockNumberNew).toBeGreaterThan(blockNumberOld)
  }, 10_000)

})
