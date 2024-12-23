import { createPublicClient, http, webSocket} from "viem"
import { mainnet, polygon } from "@gfxlabs/oku-chains"


const chains = [mainnet, polygon]

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
   test("subscription for 5 received more than one block", async ()=>{
    const receivedHeaders = []
    const subClient = await provider[polygon.id].transport.subscribe({
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
