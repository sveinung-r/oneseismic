import numpy as np
import matplotlib.pyplot as plt
import requests
import json
import argparse
import sys
import jwt
import logging

from dotenv import load_dotenv
import requests
import msal
import atexit
import os
from pathlib import Path

def get_token():
    load_dotenv()

    cache_file = os.path.join(Path.home(), ".oneseismic")
    cache = msal.SerializableTokenCache()
    if os.path.exists(cache_file):
        cache.deserialize(open(cache_file, "r").read())
    atexit.register(lambda: open(cache_file, "w").write(cache.serialize()))
    config = {
        "client_id": os.getenv("CLIENT_ID"),
        "authority": os.getenv("AUTHSERVER"),
        "scopes": ["https://storage.azure.com/user_impersonation"],
    }

    # Create a preferably long-lived app instance which maintains a token cache.
    app = msal.PublicClientApplication(
        config["client_id"], authority=config["authority"], token_cache=cache,
    )

    # The pattern to acquire a token looks like this.
    result = None

    # Note: If your device-flow app does not have any interactive ability, you can
    #   completely skip the following cache part. But here we demonstrate it anyway.
    # We now check the cache to see if we have some end users signed in before.
    accounts = app.get_accounts()
    if accounts:
        logging.info("Account(s) exists in cache, probably with token too. Let's try.")
        # print("Pick the account you want to use to proceed:")
        # for a in accounts:
        #     print(a)
        # Assuming the end user chose this one
        chosen = accounts[0]
        # Now let's try to find a token in cache for this account
        result = app.acquire_token_silent(config["scopes"], account=chosen)

    if not result:
        logging.info("No suitable token exists in cache. Let's get a new one from AAD.")

        flow = app.initiate_device_flow(config["scopes"])
        if "user_code" not in flow:
            raise ValueError(
                "Fail to create device flow. Err: %s" % json.dumps(flow, indent=4)
            )

        print(flow["message"])
        sys.stdout.flush()  # Some terminal needs this to ensure the message is shown

        # Ideally you should wait here, in order to save some unnecessary polling
        # input(
        #     "Press Enter after signing in from another device to proceed, CTRL+C to abort."
        # )

        result = app.acquire_token_by_device_flow(flow)  # By default it will block
        # You can follow this instruction to shorten the block time
        #    https://msal-python.readthedocs.io/en/latest/#msal.PublicClientApplication.acquire_token_by_device_flow
        # or you may even turn off the blocking behavior,
        # and then keep calling acquire_token_by_device_flow(flow) in your own customized loop.

    if "access_token" in result:
        # Calling graph using the access token
        return {"Authorization": "Bearer " + result["access_token"]}

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
    r = requests.get(url, headers=get_token())

    print(r.status_code)
    parts = json.loads(r.content)
    slice = assemble_slice(parts, args.shape0, args.shape1)
    plt.imshow(slice.T, cmap='seismic')
    plt.show()

if __name__ == '__main__':
    print(main(sys.argv[1:]))
