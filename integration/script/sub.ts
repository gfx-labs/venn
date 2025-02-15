import { createPublicClient, http, webSocket} from "viem"
import { mainnet, polygon } from "@gfxlabs/oku-chains"

const chains = [mainnet, polygon]

const venn_url = process.env.VENN_URL || `localhost:8545`

const provider = Object.fromEntries(chains.map((x)=>{
  const tp = webSocket(`ws://${venn_url}/${x.internalName}`)
  const pc = createPublicClient({
    chain: x,
    transport:tp,
  })
  return [x.id, pc]
}))

const subClient = provider[polygon.id].transport.subscribe({
  params:["newHeads"],
  onData: (x)=>{
    console.log(x.result.number, new Date())
  }
}).catch(console.error)
