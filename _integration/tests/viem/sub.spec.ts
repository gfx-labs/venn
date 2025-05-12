import { createPublicClient, http, webSocket} from "viem"
import { mainnet, base } from "@gfxlabs/oku-chains"


const chains = [mainnet, base]

const venn_url = process.env.VENN_URL || `localhost:8545`

const provider = Object.fromEntries(chains.map((x)=>{
  const tp = webSocket(`ws://${venn_url}/${x.internalName}`)
  const pc = createPublicClient({
    chain: x,
    transport:tp,
  })
  return [x.id, pc]
}))

describe("ws simple", ()=>{
  afterAll(async ()=>{
    for(const z of Object.values(provider)){
      await z.transport.getRpcClient().then((x)=>{
        x.close()
      })
    }
  })
  test("subscription for 5 received more than one block", async ()=>{
    const receivedHeaders = []
    const subClient = await provider[base.id].transport.subscribe({
      params:["newHeads"],
      onData: (x)=>{
        receivedHeaders.push(x.result)
      }
    })
    await new Promise(resolve => setTimeout(resolve, 5000));
    subClient.unsubscribe()
    expect(receivedHeaders.length).toBeGreaterThan(1)
  }, 10_000)
})
