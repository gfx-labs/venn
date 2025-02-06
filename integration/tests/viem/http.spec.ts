import { createPublicClient, http, webSocket} from "viem"
import { mainnet, polygon } from "@gfxlabs/oku-chains"

const chains = [mainnet, polygon]

const venn_url = process.env.VENN_URL || `localhost:8545`

describe("http simple", ()=>{
  const provider = Object.fromEntries(chains.map((x)=>{
    return [x.id,createPublicClient({
      chain: x,
      transport: http(`http://${venn_url}/${x.internalName}`),
    })]
  }))
  test("able to get mainnet block number", async ()=>{
    const blockNumber = await provider[mainnet.id].getBlockNumber()
    expect(blockNumber).toBeGreaterThan(19066961n)
  })
  test("able to get a finalized block ", async ()=>{
    const block = await provider[polygon.id].getBlock({blockTag: "finalized"})
    expect(block.number).toBeGreaterThan(19066961n)
  })
  test("polygon block changes within 5 seconds", async ()=>{
    const blockNumberOld = await provider[polygon.id].getBlockNumber()
    await new Promise(resolve => setTimeout(resolve, 5000));
    const blockNumberNew = await provider[polygon.id].getBlockNumber()
    expect(blockNumberNew).toBeGreaterThan(blockNumberOld)
  }, 10_000)
})

