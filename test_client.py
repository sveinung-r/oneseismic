import numpy as np
import matplotlib.pyplot as plt
import requests
import json
import argparse
import sys
import jwt

def assemble_slice(parts, shape0, shape1):
    tiles = parts['tiles']

    slice = np.zeros(shape0*shape1)

    for tile in tiles:
        layout = tile['layout']
        dst = layout['initial_skip']
        chunk_size = layout['chunk_size']
        src = 0
        for _ in range(layout['iterations']):
            slice[dst:dst+chunk_size] = tile['v'][src:src+chunk_size]
            src += layout['substride']
            dst += layout['superstride']

    slice = slice.reshape((shape0, shape1))

    return slice

def main(argv):
    parser = argparse.ArgumentParser('Fetch a slice')
    parser.add_argument('--base-url', type=str)
    parser.add_argument('--guid', type=str)
    parser.add_argument('--dim', type=int)
    parser.add_argument('--shape0', type=int)
    parser.add_argument('--shape1', type=int)
    parser.add_argument('--lineno', type=int)
    parser.add_argument('--secret', type=str, default="")

    args = parser.parse_args(argv)

    url = f"{args.base_url}/{args.guid}/slice/{args.dim}/{args.lineno}"
    token = jwt.encode({"exp": 1612534338}, args.secret, algorithm='HS256')
    token = token.decode('UTF-8')
    r = requests.get(url, headers={"Authorization": "Bearer " + str(token)}, verify='./aks-ingress-tls.crt')

    print(r.status_code)
    parts = json.loads(r.content)
    slice = assemble_slice(parts, args.shape0, args.shape1)
    plt.imshow(slice.T, cmap='seismic')
    plt.show()

if __name__ == '__main__':
    print(main(sys.argv[1:]))
